GO      ?= go
LDFLAGS  = -ldflags "-s -w"

.PHONY: all local armv6 arm64 clean deploy

all: local armv6 arm64

local:
	$(GO) build $(LDFLAGS) -o webterminal .

# One binary for every 32-bit Pi: Pi 1/Zero, RevPi Core 1 (ARMv6)
# and all ARMv7/ARMv8 devices running 32-bit Raspberry Pi OS (armhf).
armv6:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=6 $(GO) build $(LDFLAGS) -o webterminal-armv6 .

# For Pis running 64-bit Raspberry Pi OS.
arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(LDFLAGS) -o webterminal-arm64 .

# Usage: make deploy PI=pi@192.168.1.50 [BIN=webterminal-armv6]
BIN ?= webterminal-armv6
deploy: armv6
	scp $(BIN) webterminal.service $(PI):/tmp/
	ssh $(PI) 'sudo install -m755 /tmp/$(BIN) /usr/local/bin/webterminal && \
	           sudo install -m644 /tmp/webterminal.service /etc/systemd/system/ && \
	           sudo systemctl daemon-reload && sudo systemctl enable --now webterminal'

clean:
	rm -f webterminal webterminal-armv6 webterminal-arm64
