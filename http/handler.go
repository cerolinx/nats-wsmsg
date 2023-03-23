package http

import (
	"fmt"
	"github.com/fasthttp/router"
	"github.com/nats-io/nats.go"
	"github.com/octu0/nats-wsmsg"
	"github.com/octu0/nats-wsmsg/logger"
	"github.com/valyala/fasthttp"
	"log"
)

func Handler(natsUrl string, lg *logger.HttpLogger) fasthttp.RequestHandler {
	natsPool := NewNatsConnPool(natsUrl,
		nats.ErrorHandler(func(nc *nats.Conn, sub *nats.Subscription, err error) {
			log.Printf(
				"error: conn:%s sub:%s.%s err:%+v",
				nc.ConnectedAddr(),
				sub.Subject,
				sub.Queue,
				err,
			)
		}),
	)

	r := router.New()
	ws := r.Group("/ws")
	ws.GET("/sub/{topic}", WebsocketHandleSubscribe(natsPool))
	ws.GET("/sub/{topic}/{group}", WebsocketHandleSubscribeWithQueue(natsPool))
	ws.GET("/kv/{topic}", WebsocketHandleWatchKVStore(natsPool))
	r.POST("/pub/{topic}", HandlePublish(natsPool))
	r.POST("/kv/{topic}/{group}", HandleSetKV(natsPool))

	r.GET("/_version", HandleVersion())
	r.GET("/_chk", HandleHealthcheck())
	r.GET("/_status", HandleStatus())
	r.GET("/", HandleStatus())

	return logging(lg, r.Handler)
}

func WebsocketHandleSubscribe(natsPool *NatsConnPool) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		nc, err := natsPool.Get()
		if err != nil {
			log.Printf("error: %+v", err)
			na(ctx)
			return
		}
		defer natsPool.Put(nc)

		topic := ctx.UserValue("topic").(string)
		if err := WebsocketSubscribe(ctx, nc, topic); err != nil {
			log.Printf("error: %+v", err)
			na(ctx)
			return
		}
	}
}

func WebsocketHandleWatchKVStore(natsPool *NatsConnPool) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		nc, err := natsPool.Get()
		if err != nil {
			log.Printf("error: %+v", err)
			na(ctx)
			return
		}
		defer natsPool.Put(nc)

		topic := ctx.UserValue("topic").(string)
		if err := WebsocketSubscribeKV(ctx, nc, topic); err != nil {
			log.Printf("error: %+v", err)
			na(ctx)
			return
		}
	}
}

func WebsocketHandleSubscribeWithQueue(natsPool *NatsConnPool) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		nc, err := natsPool.Get()
		if err != nil {
			log.Printf("error: %+v", err)
			na(ctx)
			return
		}
		defer natsPool.Put(nc)

		topic := ctx.UserValue("topic").(string)
		group := ctx.UserValue("group").(string)
		if err := WebsocketSubscribeWithQueue(ctx, nc, topic, group); err != nil {
			log.Printf("error: %+v", err)
			na(ctx)
			return
		}
	}
}

func HandlePublish(natsPool *NatsConnPool) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		nc, err := natsPool.Get()
		if err != nil {
			log.Printf("error: %+v", err)
			na(ctx)
			return
		}
		defer natsPool.Put(nc)

		topic := ctx.UserValue("topic").(string)
		nc.Publish(topic, ctx.PostBody())
	}
}

func HandleSetKV(natsPool *NatsConnPool) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		log.Printf("infoo: KV req -----------")
		nc, err := natsPool.Get()
		if err != nil {
			log.Printf("error: %+v", err)
			na(ctx)
			return
		}
		defer natsPool.Put(nc)

		topic := ctx.UserValue("topic").(string)
		group := ctx.UserValue("group").(string)
		log.Printf("info: KV try  %s   %s", topic, group)
		//
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
		keys, _ := kv.Keys()
		fmt.Printf("debug: keys for store %s %+q\n", topic, keys)
		kv.Put(group, ctx.PostBody())
		entry, _ := kv.Get(group)
		fmt.Printf("info:  %s @ %d -> %q\n", entry.Key(), entry.Revision(), string(entry.Value()))
	}
}

func HandleVersion() fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		ok(ctx, wsmsg.Version)
	}
}

func HandleHealthcheck() fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		ok(ctx, `OK`)
	}
}

func HandleStatus() fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		ok(ctx, `OK`)
	}
}

func writeln(ctx *fasthttp.RequestCtx, text string) {
	ctx.Write([]byte(text))
	ctx.Write([]byte("\n"))
}

func na(ctx *fasthttp.RequestCtx) {
	ctx.SetContentType("application/json")
	ctx.SetStatusCode(fasthttp.StatusNotAcceptable)

	writeln(ctx, `{"status":"not acceptable"}`)
}

func ok(ctx *fasthttp.RequestCtx, text string) {
	ctx.SetContentType("text/plain")
	ctx.SetStatusCode(fasthttp.StatusOK)

	writeln(ctx, text)
}

func logging(lg *logger.HttpLogger, handler fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		handler(ctx)

		lg.Accesslog(
			ctx.URI().Host(),
			ctx.URI().RequestURI(),
			ctx.Request.Header.Method(),
			ctx.Request.Header.UserAgent(),
			ctx.Response.Header.StatusCode(),
		)
	}
}
