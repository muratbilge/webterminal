GO      ?= go
LDFLAGS  = -ldflags "-s -w"

.PHONY: all local armv6 arm64 amd64 clean deploy FORCE

all: local armv6 arm64 amd64

local:
	$(GO) build $(LDFLAGS) -o webterminal .

armv6: webterminal-armv6
arm64: webterminal-arm64
amd64: webterminal-amd64

# One binary for every 32-bit Pi: Pi 1/Zero, RevPi Core 1 (ARMv6)
# and all ARMv7/ARMv8 devices running 32-bit Raspberry Pi OS (armhf).
webterminal-armv6: FORCE
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=6 $(GO) build $(LDFLAGS) -o $@ .

# For Pis running 64-bit Raspberry Pi OS.
webterminal-arm64: FORCE
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(LDFLAGS) -o $@ .

# For x86-64 Linux servers.
webterminal-amd64: FORCE
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -o $@ .

FORCE:

# Usage: make deploy PI=pi@192.168.1.50 [BIN=webterminal-arm64]
BIN ?= webterminal-armv6
deploy: $(BIN)
	scp $(BIN) webterminal.service $(PI):/tmp/
	ssh $(PI) 'sudo install -m755 /tmp/$(BIN) /usr/local/bin/webterminal && \
	           sudo install -m644 /tmp/webterminal.service /etc/systemd/system/ && \
	           sudo systemctl daemon-reload && sudo systemctl enable --now webterminal'

clean:
	rm -f webterminal webterminal-armv6 webterminal-arm64 webterminal-amd64
