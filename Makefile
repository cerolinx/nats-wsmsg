VERSION_GO = version.go
MAIN_GO    = cmd/main.go 

_NAME      = $(shell grep -o 'AppName string = "[^"]*"' $(VERSION_GO)  | cut -d '"' -f2)
_VERSION   = $(shell grep -o 'Version string = "[0-9]\.[0-9]\.[0-9]"' $(VERSION_GO) | cut -d '"' -f2)

_GOOS      = linux
_GOARCH    = amd64

build:
	GOOS=$(_GOOS) GOARCH=$(_GOARCH) GO111MODULE=on go build -o $(_NAME) $(MAIN_GO)

test: deps
	go test -v ./...

install: deps
	go install

pkg-build      = GOOS=$(1) GOARCH=$(2) go build -o pkg/$(3)_$(1)_$(2)-$(_VERSION) $(4)
pkg-build-main = $(call pkg-build,$(1),$(2),$(_NAME),$(MAIN_GO))

zip            = cp pkg/$(3)_$(1)_$(2)-$(_VERSION) pkg/$(3) && zip -j pkg/$(3)_$(1)_$(2)-$(_VERSION).zip pkg/$(3) && rm pkg/$(3)
zip-main       = $(call zip,$(1),$(2),$(_NAME))

pre-pkg:
	go generate
	mkdir -p pkg

pkg-linux-amd64:
	$(call pkg-build-main,linux,amd64)
	$(call zip-main,linux,amd64)

pkg-darwin-amd64:
	$(call pkg-build-main,darwin,amd64)
	$(call zip-main,darwin,amd64)

pkg: pre-pkg \
	pkg-linux-amd64 \
	pkg-darwin-amd64

clean:
	rm -f $(_NAME)
	rm -f pkg/*
