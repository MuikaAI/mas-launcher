# mas-launcher

独立的 Muika-After-Story 跨平台单文件启动器：负责拉取主项目、准备 Python 环境，并管理每个实例的 Core / Bot 进程。

## 构建

```bash
go test ./...
go build -trimpath -ldflags "-s -w -X main.version=dev" -o mas-launcher .
```

## 快速开始

```bash
mas-launcher                          # 无参数：自动 初始化 → 配置 → 选模型 → 启动 default 实例
mas-launcher init                     # 创建默认实例（克隆主项目 + 准备 Python 环境）
mas-launcher configure                # 配置 .env（Master ID、IPC 密钥、WebSocket 地址）
mas-launcher model                    # 配置 models.yml（选 provider → 拉模型列表 → 选模型）
mas-launcher start                    # 首次启动会先展示许可协议，同意后拉起 Core 与 Bot
```

`init` 创建实例时会优先使用 `git clone`（因此实例之后可以用 `update` 更新）；Python 环境优先用 uv，未安装时回退到系统 Python。

## 命令

```
mas-launcher                                       bootstrap default instance (init/configure/model/start)
mas-launcher init [name]                         clone and prepare an instance
mas-launcher configure [name]                    configure .env and model
mas-launcher model [name]                        configure models.yml (wizard or CRUD)
mas-launcher license [name] [--status]           view/sign the license agreement
mas-launcher start [name] [--foreground]         start Core and Bot
mas-launcher stop|restart [name]                 manage processes
mas-launcher status [name] [--json]              inspect state
mas-launcher logs [name] [--service core|bot]    view logs
mas-launcher update [name] [--ref REF]           update the checkout
mas-launcher doctor [name]                       diagnose environment
mas-launcher remove [name]                       remove an instance
```

### 常用命令说明

- **`（无参数）`** — 对 `default` 实例自动引导：实例不存在则先 `init`；依次检查 Python 环境、`.env`、`MASTER_ID`、`models.yml` 默认模型，缺失项自动修复或进入对应向导；若已在运行则提示后退出，否则调用 `start` 启动 Core 与 Bot。`help` / `--help` / `-h` 仍打印帮助，`version` / `--version` 仍打印版本。
- **`init [name]`** — 创建并准备一个实例（默认名 `default`）：拉取主项目、初始化 `.env`、创建 Python venv。
- **`configure [name]`** — 交互设置 `.env`（Master ID、IPC_SECRET、CORE_WS_URL）。模型配置请改用 `model` 命令。
- **`model [name]`** — 配置 `configs/models.yml` 的向导：内嵌 openai / kimi / glm / deepseek / dashscope / gemini / ollama 或自定义 `api_host`，从 provider 拉取模型列表供选择，再配置 api_key 与采样参数；已有配置时进入 新建/修改/删除 菜单。也支持脚本化参数：`--list`、`--delete NAME --yes`、`--set-default NAME`，以及 `--name ... --provider ... --model ... --api-key ...` 直接写入。
- **`license [name]`** — 查看 / 签署许可协议（`--status` 仅打印状态）。`start` 前会自动检查：未签署或版本过旧时在启动器终端内展示条款等待同意，拒绝则中止启动。
- **`start [name] [--foreground]`** — 后台启动 Core 与 Bot；`--foreground` 前台运行，`--no-bot` 只启动 Core。
- **`stop` / `restart [name]`** — 停止 / 重启实例。
- **`status [name] [--json]`** — 查看进程状态。
- **`logs [name] [--service core|bot] [-f]`** — 查看 Core / Bot 日志，`-f` 跟随输出。
- **`update [name] [--ref REF]`** — 更新实例代码（要求实例是 git 检出）。
- **`doctor [name]`** — 诊断环境（项目文件、Python、`.env` 是否就绪）。
- **`remove [name]`** — 删除实例（需输入 `yes` 确认）。

## 数据位置

实例、日志、配置保存在平台用户数据目录：

| 平台 | 路径 |
|------|------|
| Windows | `%LOCALAPPDATA%\Muika-After-Story` |
| macOS | `~/Library/Application Support/Muika-After-Story` |
| Linux | `~/.local/share/muika-after-story` |

可用环境变量 `MUIKA_HOME` 覆盖。

## 语言

启动器界面默认按 `MUIKA_LANG` > `LC_ALL` > `LANG` > 系统 UI 语言的优先级自动选择中英文。设置 `MUIKA_LANG=zh` 强制中文，`MUIKA_LANG=en` 强制英文（英文为内置回退语言）。

## 环境要求

- **Go ≥ 1.22**（仅构建时）
- 运行实例时：**Python ≥ 3.10**（或 uv）、**Git**（克隆与更新实例用）
