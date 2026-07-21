# Java codegen is handled by the protobuf-gradle-plugin automatically on
# `./gradlew build`, it reads java-matcher/src/main/proto directly. Only
# the Go side needs an explicit protoc invocation here.

PROTO_DIR := proto
GO_OUT := go-client/gen/biometricpb

.PHONY: proto
proto:
	@echo "Generating Go gRPC stubs from $(PROTO_DIR)/biometric.proto"
	protoc \
		--go_out=$(GO_OUT) --go_opt=paths=source_relative \
		--go-grpc_out=$(GO_OUT) --go-grpc_opt=paths=source_relative \
		-I $(PROTO_DIR) \
		$(PROTO_DIR)/biometric.proto

# One-time local setup, not something CI should need since the binaries
# should already be on the CI image.
.PHONY: install-tools
install-tools:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest