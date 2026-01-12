.PHONY: run swagger

# 运行服务
run:
	go run cmd/main.go

# 生成 Swagger 文档
swagger:
	~/go/bin/swag init -g cmd/main.go --output docs
