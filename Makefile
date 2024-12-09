PROTOC        = protoc
PROTO_DIR     = api
OUT_DIR       = api

# 自动收集 api 目录下的所有 .proto 文件
PROTO_FILES   = $(wildcard $(PROTO_DIR)/*.proto)

# 编译选项
GO_OUT_OPT    = --go_out=$(OUT_DIR) --go_opt=paths=source_relative
GRPC_OUT_OPT  = --go-grpc_out=$(OUT_DIR) --go-grpc_opt=paths=source_relative

# 默认目标
all: gen_proto

# 生成 Protobuf 文件
gen_proto:
	$(PROTOC) --proto_path=$(PROTO_DIR) \
	$(GO_OUT_OPT) \
	$(GRPC_OUT_OPT) \
	$(PROTO_FILES)

# make gen_single file=api/common.proto
gen_single:
	$(PROTOC) --proto_path=$(PROTO_DIR) \
	$(GO_OUT_OPT) \
	$(GRPC_OUT_OPT) \
	$(file)


clean:
	find $(OUT_DIR) -name "*.pb.go" -delete

.PHONY: all gen_proto clean
