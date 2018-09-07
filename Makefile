
GO ?= go

name := hercules
target_path := target
binary_path := $(target_path)/bin/$(name)
config_path := $(target_path)/etc/$(name).config.json
snapshot_path := $(target_path)/snapshots/latest

all: $(binary_path)

$(binary_path):
	$(GO) build -v -o $(binary_path) hercules.go

$(config_path): $(name).config.json
	mkdir -p `dirname $(config_path)`
	cp $< $(config_path)

run: $(binary_path) $(config_path)
	mkdir -p $(target_path)/data
	cd $(target_path) && bin/$(name) -c etc/$(name).config.json

clean:
	rm -f $(binary_path) $(config_path)

.SILENT: download-snapshot
download-snapshot:
	mkdir -p `dirname $(snapshot_path)`
	./download_snapshot.sh $(snapshot_path)
