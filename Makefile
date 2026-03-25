.PHONY: proto build run clean

proto:
	protoc --go_out=proto/protocol --go_opt=paths=source_relative \
	       --go-grpc_out=proto/protocol --go-grpc_opt=paths=source_relative \
	       -I proto/ proto/ui.proto

build:
	go build -o ostui .

run: build
	./ostui

clean:
	rm -f ostui
