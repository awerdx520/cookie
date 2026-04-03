BINARY   := cookie-cli
MODULE   := cookie
SRC      := ./cmd/cookie-cli
EXT_DIR  := extension
EXT_DEST := /mnt/c/Users/$(shell /mnt/c/Windows/System32/cmd.exe /c "echo %USERNAME%" 2>/dev/null | tr -d '\r\n')/cookie-bridge-extension

GO       := go
GOFLAGS  := -trimpath
PORT     ?= 8008

.PHONY: all build run serve install clean fmt vet test ext-install ext-copy help

all: build

## ── 构建 ─────────────────────────────────────────────

build:  ## 编译 cookie-cli
	$(GO) build $(GOFLAGS) -o $(BINARY) $(SRC)

## ── 运行 ─────────────────────────────────────────────

run: build  ## 编译并启动 Bridge 服务
	./$(BINARY) serve -port $(PORT)

serve: build  ## 同 run
	./$(BINARY) serve -port $(PORT)

## ── 扩展 ─────────────────────────────────────────────

ext-copy:  ## 复制 Chrome 扩展到 Windows 用户目录
	@mkdir -p "$(EXT_DEST)"
	cp $(EXT_DIR)/* "$(EXT_DEST)/"
	@echo "扩展已复制到 $(EXT_DEST)"
	@echo "请在 Chrome 中加载: chrome://extensions → 开发者模式 → 加载已解压的扩展程序"

## ── 安装 ─────────────────────────────────────────────

install: build  ## 安装到 GOPATH/bin
	$(GO) install $(GOFLAGS) $(SRC)

## ── 代码质量 ─────────────────────────────────────────

fmt:  ## 格式化代码
	$(GO) fmt ./...

vet:  ## 静态检查
	$(GO) vet ./...

test:  ## 运行测试
	$(GO) test ./...

## ── 清理 ─────────────────────────────────────────────

clean:  ## 清理构建产物
	rm -f $(BINARY)
	$(GO) clean

## ── 帮助 ─────────────────────────────────────────────

help:  ## 显示帮助
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'
