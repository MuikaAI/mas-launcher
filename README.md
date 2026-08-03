# mas-launcher

独立的 Muika-After-Story 跨平台单文件启动器。

```bash
go test ./...
go build -trimpath -ldflags "-s -w -X main.version=dev" -o mas-launcher .
```

首次 `mas-launcher init` 会自动获取主项目；优先使用 uv，未安装时回退到系统 Python。实例、日志和配置保存在平台用户数据目录，可用 `MUIKA_HOME` 覆盖。
