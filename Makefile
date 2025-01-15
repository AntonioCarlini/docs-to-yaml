.DEFAULT: all

# Makefile targets:
#
# all:
#    Build the necessary results tree under bin/
#    Build all main golang programs
#
# yaml:
#    Build the 'all' target
#    Build bitsavers & vaxhaven YAML output
#
define go_build
bin/$(1): $(1)/$(1).go
	go build -o bin/$(1) $(1)/$(1).go

endef

GO_PROGRAMS += bitsavers-to-yaml
GO_PROGRAMS += file-tree-to-yaml
GO_PROGRAMS += local-archive-to-yaml
GO_PROGRAMS += vaxhaven-to-yaml
GO_PROGRAMS += yaml-to-csv

YAML_OUTPUT += bin/yaml/bitsavers.yaml
YAML_OUTPUT += bin/yaml/vaxhaven.yaml

BIN_DIR = bin

.PHONY: | build.tree

build.tree:
	@mkdir -p $(BIN_DIR)
	@mkdir -p $(BIN_DIR)/yaml

all: build.tree

all: $(foreach PRG,$(GO_PROGRAMS),bin/$(PRG))

all: $(eval $(foreach PRG,$(GO_PROGRAMS),$(call go_build,$(PRG))))

bin/yaml/bitsavers.yaml: bin/bitsavers-to-yaml data/VaxHaven.txt
	bin/bitsavers-to-yaml --yaml-output bin/yaml/bitsavers.yaml

bin/yaml/vaxhaven.yaml: bin/vaxhaven-to-yaml data/bitsavers-IndexByDate.txt
	bin/vaxhaven-to-yaml  --yaml-output bin/yaml/vaxhaven.yaml

yaml: all

yaml: $(YAML_OUTPUT)
