
GO ?= go
DOCKER ?= docker

name := hercules
target_path := target
binary_path := $(target_path)/bin/$(name)
config_path := $(target_path)/etc/$(name).config.json
snapshot_path := $(target_path)/latest

all: $(binary_path)

$(binary_path):
	CGO_ENABLED=0 $(GO) build -a -installsuffix cgo -ldflags="-w -s" -o $(binary_path) hercules.go

$(config_path): $(name).config.json
	mkdir -p `dirname $(config_path)`
	cp $< $(config_path)

test:
	$(GO) test -v ./...

run: $(binary_path) $(config_path)
	rm -rf $(target_path)/data
	mkdir -p $(target_path)/data
	mkdir -p $(target_path)/snapshots
	cd $(target_path) && bin/$(name) -c etc/$(name).config.json --snapshots.loadFile latest.snap

image: $(binary_path)
	$(DOCKER) build -t $(name) .

run-image: image
	$(DOCKER) run -it --rm -v /tmp:/tmp -v `pwd`/target/latest.snap:/latest.snap $(name) --snapshots.loadFile /latest.snap

clean:
	rm -f $(binary_path) $(config_path)

.SILENT: download-snapshot
download-snapshot:
	./download_snapshot.sh $(snapshot_path)
