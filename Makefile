#
# Gisp interpreter
#
PROG    = floppy
DESTDIR	= $(HOME)/.local

.PHONY: all install uninstall clean test cover bench gotestsum source

all:
	go build

install: all
	@install -d $(DESTDIR)/usr/bin
	install -m 555 ${PROG} $(DESTDIR)/usr/bin/${PROG}

uninstall:
	rm -f $(DESTDIR)/bin/${PROG}

clean:
	go clean
	rm -f ${PROG}

#
# For testing, please install gotestsum:
#	go install gotest.tools/gotestsum@latest
#
test: gotestsum
	gotestsum --format dots -- ./...

cover: gotestsum
	gotestsum -- -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

gotestsum:
	@command -v gotestsum >/dev/null || go install gotest.tools/gotestsum@latest
