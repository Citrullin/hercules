
GO ?= go

name := hercules
binary := build/$(name)

all: $(binary)

$(binary):
	$(GO) build -v -o $(binary) hercules.go

clean:
	rm -f $(binary)
