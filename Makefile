PROTO_PYTHON_FILES := $(shell find python/ -type f -name '*_pb2*.py' -printf '%p ')
PYTHON_FILES := $(filter-out $(PROTO_PYTHON_FILES), $(shell find python/ -type f -name '*.py' -printf '%p '))
GO_PKGS := $(shell go list ./... | grep -v /vendor)
GO_BIN := $(GOPATH)/bin
GOMETALINTER := $(GO_BIN)/gometalinter
GODEP := $(GO_BIN)/dep
PROTOC_GEN_GO := $(GO_BIN)/protoc-gen-go
PROTOC_GEN_GO_SRC := vendor/github.com/golang/protobuf/protoc-gen-go
GRPCIO_VERSION := 1.11.0

OUTDIR := out/
QMSTR_GO_ANALYZERS := $(foreach ana, $(shell ls cmd/analyzers), ${OUTDIR}analyzers/$(ana))
QMSTR_GO_REPORTERS := $(foreach rep, $(shell ls cmd/reporters), ${OUTDIR}reporters/$(rep))
QMSTR_GO_BUILDERS := $(foreach builder, qmstr-wrapper, ${OUTDIR}$(builder))
QMSTR_CLIENT_BINARIES := $(foreach cli, qmstr-cli qmstr, ${OUTDIR}$(cli)) $(QMSTR_GO_BUILDERS)
QMSTR_MASTER := $(foreach bin, qmstr-master, ${OUTDIR}$(bin))
QMSTR_PYTHON_SPDX_ANALYZER := ${OUTDIR}pyqmstr-spdx-analyzer
QMSTR_PYTHON_MODULES := $(QMSTR_PYTHON_SPDX_ANALYZER)
QMSTR_SERVER_BINARIES := $(QMSTR_MASTER) $(QMSTR_GO_ANALYZERS) $(QMSTR_GO_REPORTERS) $(QMSTR_PYTHON_MODULES)
QMSTR_GO_BINARIES := $(QMSTR_MASTER) $(QMSTR_CLIENT_BINARIES) $(QMSTR_GO_ANALYZERS) $(QMSTR_GO_REPORTERS)

CONTAINER_TAG_DEV := qmstr/dev
CONTAINER_TAG_MASTER := qmstr/master
CONTAINER_TAG_RUNTIME := qmstr/runtime
CONTAINER_TAG_BUILDER:= qmstr/master_build

.PHONY: all
all: $(QMSTR_GO_BINARIES) $(QMSTR_PYTHON_MODULES)

generate: go_proto python_proto

venv: venv/bin/activate
venv/bin/activate: requirements.txt
	test -d venv || virtualenv venv
	venv/bin/pip install -Ur requirements.txt
	touch venv/bin/activate

requirements.txt:
	echo grpcio-tools==$(GRPCIO_VERSION) >> requirements.txt
	echo pex >> requirements.txt
	echo autopep8 >> requirements.txt

go_proto: $(PROTOC_GEN_GO)
	protoc -I proto --go_out=plugins=grpc:pkg/service proto/*.proto

python_proto: venv
	@mkdir python/pyqmstr/service || true
	venv/bin/python -m grpc_tools.protoc -Iproto --python_out=./python/pyqmstr/pyqmstr/service --grpc_python_out=./python/pyqmstr/pyqmstr/service proto/*.proto

.PHONY: clean
clean:
	@rm $(PROTO_PYTHON_FILES) || true
	@rm pkg/service/*.pb.go || true
	@rm -r out || true
	@rm -fr venv || true
	@rm requirements.txt || true

.PHONY: cleanall
cleanall: clean
	@docker rmi ${CONTAINER_TAG_DEV} ${CONTAINER_TAG_MASTER} ${CONTAINER_TAG_RUNTIME} || true
	@docker image prune --all --force --filter=label=org.qmstr.image

.PHONY: checkpep8
checkpep8: $(PYTHON_FILES) venv
	venv/bin/autopep8 --diff $(filter-out venv, $^)

.PHONY: autopep8
autopep8: $(PYTHON_FILES) venv
	venv/bin/autopep8 -i $(filter-out venv, $^)

.PHONY: gotest
gotest: go_proto
	go test $(GO_PKGS)

$(GOMETALINTER):
	go get -u github.com/alecthomas/gometalinter
	gometalinter --install &> /dev/null

.PHONY: golint
golint:	$(GOMETALINTER)
	gometalinter ./... --vendor

$(GODEP):
	go get -u -v github.com/golang/dep/cmd/dep

.PHONY: godep
godep: $(GODEP)
	dep ensure

$(PROTOC_GEN_GO_SRC): godep

$(PROTOC_GEN_GO): $(PROTOC_GEN_GO_SRC) 
	(cd ${PROTOC_GEN_GO_SRC} && go install)

$(QMSTR_GO_BINARIES): go_proto gotest
	go build -o $@ github.com/QMSTR/qmstr/cmd/$(subst $(OUTDIR),,$@)

.PHONY: container
container: ci/Dockerfile
	docker build -f ci/Dockerfile -t ${CONTAINER_TAG_MASTER} --target master .

.PHONY: devcontainer
devcontainer: container
	docker build -f ci/Dockerfile -t ${CONTAINER_TAG_DEV} --target dev .

.PHONY: democontainer
democontainer: container
	docker build -f ci/Dockerfile -t ${CONTAINER_TAG_RUNTIME} --target runtime .
	docker build -f ci/Dockerfile -t ${CONTAINER_TAG_BUILDER} --target builder .

.PHONY: pyqmstr-spdx-analyzer
pyqmstr-spdx-analyzer: $(QMSTR_PYTHON_SPDX_ANALYZER)

$(QMSTR_PYTHON_SPDX_ANALYZER): python_proto
	venv/bin/pex ./python/pyqmstr ./python/spdx-analyzer 'grpcio==${GRPCIO_VERSION}' -v -e spdxanalyzer.__main__:main -o $@

python_modules: $(QMSTR_PYTHON_MODULES)

install_python_modules: $(QMSTR_PYTHON_MODULES)
	cp $^ /usr/local/bin

install_qmstr_server: $(QMSTR_SERVER_BINARIES)
	cp $^ /usr/local/bin

install_qmstr_client: $(QMSTR_CLIENT_BINARIES)
	cp $^ /usr/local/bin

install_qmstr_all: install_qmstr_client install_qmstr_server