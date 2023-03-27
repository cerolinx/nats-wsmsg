package http

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/nats-io/nats.go"
	"github.com/valyala/fasthttp"
)

var Origins []string

var upgrader = websocket.FastHTTPUpgrader{
	WriteBufferPool:   new(sync.Pool),
	ReadBufferSize:    4 * 1024,
	WriteBufferSize:   4 * 1024,
	EnableCompression: false,
	//CheckOrigin:       func(ctx *fasthttp.RequestCtx) bool { return true },
	CheckOrigin: func(ctx *fasthttp.RequestCtx) bool {
		if Origins == nil {
			log.Printf("debug: allowed origins %s", "*")
			return true
		}
		var origin = string(ctx.Request.Header.Peek("origin"))
		log.Printf("debug: reported origin %s", origin)
		log.Printf("debug: allowed origin %v", Origins)
		for _, allowOrigin := range Origins {
			if origin == allowOrigin {
				return true
			}
		}
		return false
	},
}

func WebsocketSubscribe(rctx *fasthttp.RequestCtx, nc *nats.Conn, topic string) error {
	return upgrader.Upgrade(rctx, func(conn *websocket.Conn) {
		defer conn.Close()

		ctx, cancel := context.WithCancel(rctx)
		defer cancel()

		queue := make(chan []byte, 64)

		sub, err := nc.Subscribe(topic, func(msg *nats.Msg) {
			queue <- msg.Data
		})
		if err != nil {
			log.Printf("error: %+v", err)
			return
		}
		defer sub.Unsubscribe()
		nc.Flush()

		go wsWriteLoop(ctx, conn, queue)
		wsReadLoop(ctx, cancel, conn)
	})
}

func WebsocketSubscribeKV(rctx *fasthttp.RequestCtx, nc *nats.Conn, topic string) error {
	return upgrader.Upgrade(rctx, func(conn *websocket.Conn) {
		defer conn.Close()

		ctx, cancel := context.WithCancel(rctx)
		defer cancel()

		queue := make(chan []byte, 64)

		js, err := nc.JetStream()
		if err != nil {
			log.Printf("error: %+v", err)
			return
		}

		var kv nats.KeyValue
		if stream, _ := js.StreamInfo("KV_" + topic); stream == nil {
			log.Printf("debug: KV %s found", topic)
			// A key-value (KV) bucket is created by specifying a bucket name.
			kv, _ = js.CreateKeyValue(&nats.KeyValueConfig{
				Bucket: topic,
			})
		} else {
			log.Printf("debug: KV %s not found", topic)
			kv, _ = js.KeyValue(topic)
		}

		//keys, _ := kv.Keys()
		//fmt.Printf("%+q\n", keys)
		//for _, key := range keys {
		//	log.Printf("info: getting key: %s", topic+"."+key)
		//	entry, _ := kv.Get(key)
		//	fmt.Printf("%s @ %d -> %q\n", entry.Key(), entry.Revision(), string(entry.Value()))
		//	queue <- entry.Value()
		//}

		w, _ := kv.WatchAll(nats.IgnoreDeletes())
		defer w.Stop()

		go kvWatchLoop(ctx, w, queue)

		go wsWriteLoop(ctx, conn, queue)
		wsReadLoop(ctx, cancel, conn, kv)
	})
}

func kvWatchLoop(ctx context.Context, w nats.KeyWatcher, queue chan []byte) {
	for {
		select {
		case <-ctx.Done():
			return
		case kve := <-w.Updates():
			if kve != nil {
				log.Printf("debug: %s @ %d -> %q (op: %s)\n", kve.Key(), kve.Revision(), string(kve.Value()), kve.Operation())
				queue <- []byte("{\"" + kve.Key() + "\":" + string(kve.Value()) + "}")
			}
		}
	}
}

func WebsocketSubscribeWithQueue(rctx *fasthttp.RequestCtx, nc *nats.Conn, topic, group string) error {
	return upgrader.Upgrade(rctx, func(conn *websocket.Conn) {
		defer conn.Close()

		ctx, cancel := context.WithCancel(rctx)
		defer cancel()

		queue := make(chan []byte, 64)

		sub, err := nc.QueueSubscribe(topic, group, func(msg *nats.Msg) {
			queue <- msg.Data
		})
		if err != nil {
			log.Printf("error: %+v", err)
			return
		}
		defer sub.Unsubscribe()
		nc.Flush()

		go wsWriteLoop(ctx, conn, queue)
		wsReadLoop(ctx, cancel, conn)
	})
}

func wsWriteLoop(ctx context.Context, conn *websocket.Conn, queue chan []byte) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("error: %+v", err)
				return
			}

		case data := <-queue:
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			w, err := conn.NextWriter(websocket.TextMessage)
			if err != nil {
				log.Printf("error: %+v", err)
				return
			}
			w.Write(data)

			if err := w.Close(); err != nil {
				log.Printf("error: %+v", err)
				return
			}
		}
	}
}

func wsReadLoop(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, kv ...nats.KeyValue) {
	defer cancel()

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		return nil
	})
	for {
		select {
		case <-ctx.Done():
			return // maybe http conn timeout?
		default:
			// nop
		}

		typ, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %+v", err)
			}
			return
		}
		log.Printf("info: Recieved message type %d: %s", typ, msg)
		if len(kv) == 1 && typ == 1 {
			var order map[string]string

			err = json.Unmarshal(msg, &order)

			//Checks whether the error is nil or not
			if err != nil {

				//Prints the error if not nil
				log.Printf("error: while decoding the data %+v", err)
			}
			value, checkVariableName := order["delete"]
			if checkVariableName {
				log.Printf("info: deleting key %s", value)
				kv[0].Purge(value)
				kv[0].PurgeDeletes(nats.DeleteMarkersOlderThan(-1))
			}
		}
	}
}
