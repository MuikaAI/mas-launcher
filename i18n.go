package main

import (
	"fmt"
	"os"
	"strings"
)

// zhMessages is the Chinese translation catalog: English message key -> Chinese
// translation. Format strings keep the exact printf verb sequence of the key
// (enforced by the consistency test in i18n_test.go).
var zhMessages = map[string]string{
	// main.go
	"instance is not running":                    "实例未在运行",
	"unknown command %q; run mas-launcher help":  "未知命令 %q；运行 mas-launcher help 获取帮助",
	"instance %q does not exist; run init first": "实例 %q 不存在；请先运行 init",
	`Muika-After-Story launcher

  mas-launcher                                      bootstrap default instance (init/configure/model/start)
  mas-launcher init [name]                         clone and prepare an instance
  mas-launcher configure [name]                    configure .env and model
  mas-launcher model [name]                        configure models.yml (wizard or CRUD)
  mas-launcher license [name] [--status]           view/sign the license agreement
  mas-launcher napcat [name] [--show-napcat] [--admin] [--stop] configure QQ access (NapCat)
  mas-launcher start [name] [--foreground]         start Core and Bot
  mas-launcher stop|restart [name]                 manage processes
  mas-launcher status [name] [--json]              inspect state
  mas-launcher logs [name] [--service core|bot]    view logs
  mas-launcher update [name] [--ref REF]           update the checkout
  mas-launcher doctor [name]                       diagnose environment
  mas-launcher remove [name]                       remove an instance`: `Muika-After-Story 启动器

  mas-launcher                                      引导默认实例（init/configure/model/start）
  mas-launcher init [name]                         克隆并准备实例
  mas-launcher configure [name]                    配置 .env 与模型
  mas-launcher model [name]                        配置 models.yml（向导或增删改）
  mas-launcher license [name] [--status]           查看/签署许可协议
  mas-launcher napcat [name] [--show-napcat] [--admin] [--stop] 配置 QQ 接入（NapCat）
  mas-launcher start [name] [--foreground]         启动 Core 与 Bot
  mas-launcher stop|restart [name]                 管理进程
  mas-launcher status [name] [--json]              查看状态
  mas-launcher logs [name] [--service core|bot]    查看日志
  mas-launcher update [name] [--ref REF]           更新代码
  mas-launcher doctor [name]                       诊断环境
  mas-launcher remove [name]                       删除实例`,

	// commands.go
	"Fetching project...": "正在拉取项目...",
	"Instance %s created. Run: mas-launcher configure %s\n":            "实例 %s 已创建。运行：mas-launcher configure %s\n",
	"Installing dependencies with uv...":                               "正在用 uv 安装依赖...",
	"uv not found; using system Python...":                             "未找到 uv，改用系统 Python...",
	"Configuration saved. Add model settings with: mas-launcher model": "配置已保存。使用 mas-launcher model 添加模型设置",
	"%s started (PID %d)\n":                                            "%s 已启动（PID %d）\n",
	"Core":                                                             "Core",
	"Bot":                                                              "Bot",
	"Instance %s is running in background.\n":                          "实例 %s 正在后台运行。\n",
	"Instance %s stopped.\n":                                           "实例 %s 已停止。\n",
	"Instance: %s\nPath: %s\nCore: %v (PID %d)\nBot: %v (PID %d)\nCommit: %s\n": "实例: %s\n路径: %s\nCore: %v (PID %d)\nBot: %v (PID %d)\n提交: %s\n",
	"%s: ok\n":                   "%s: 正常\n",
	"%s: missing\n":              "%s: 缺失\n",
	"project":                    "项目",
	"Python":                     "Python",
	"config":                     "配置",
	"Type yes to remove %s: ":    "输入 yes 以删除 %s：",
	"Master ID":                  "Master ID",
	"IPC_SECRET":                 "IPC_SECRET",
	"Core WebSocket URL":         "Core WebSocket 地址",
	"instance %q already exists": "实例 %q 已存在",
	"uv or Python 3.10+ is required; install uv and retry":    "需要 uv 或 Python 3.10+；请安装 uv 后重试",
	"--provider, --model and --api-key are required together": "--provider、--model 和 --api-key 必须同时提供",
	"Python environment missing; run init first":              "Python 环境缺失；请先运行 init",
	"instance is already running":                             "实例已在运行",
	"service must be core or bot":                             "service 必须是 core 或 bot",
	"stop the instance before updating":                       "更新前请先停止实例",
	"working tree is dirty; update refused":                   "工作区有未提交改动；已拒绝更新",
	"Git is required for update":                              "更新需要 Git",
	"stop the instance before removing it":                    "删除前请先停止实例",
	"cancelled":                                               "已取消",

	// default.go
	"No default instance found; creating one...": "未找到默认实例，正在创建...",
	"Instance already running.":                  "实例已在运行。",
	"  ok: %s\n":                                 "  %s 就绪\n",
	"  %s missing; fixing...\n":                  "  %s 缺失，正在修复...\n",
	"Warning: still missing: %v\n":               "警告：仍有缺失：%v\n",
	"python":                                     "Python 环境",
	"env-file":                                   ".env 文件",
	"master-id":                                  "Master ID",
	"model":                                      "模型配置",

	// license.go
	"Agree? (yes/no): ": "同意吗？(是/否): ",
	"The terms were updated on %s. You must agree to the terms and read the license declaration before continuing to use MAS\n": "以上条款更新于 %s。您必须同意以上条款和阅读许可证声明后才可继续使用 MAS\n",
	"Thank you for agreeing. MAS will now start running":                                                                        "感谢您的同意，MAS 将开始运行",
	"You did not agree to the agreement; MAS cannot continue running":                                                           "您未同意协议，MAS 无法继续运行",
	"agreement file has empty title/text/updated":                                                                               "协议文件缺少 title/text/updated 字段",
	"cannot read %s: %w — run 'mas-launcher update' to fetch it":                                                                "无法读取 %s：%w — 请运行 'mas-launcher update' 获取它",
	"warning: corrupt agreement state %s (%v); will re-prompt\n":                                                                "警告：协议状态文件 %s 损坏（%v）；将重新提示\n",
	"License accepted (version %s, signed %s).\n":                                                                               "许可已接受（版本 %s，签署于 %s）。\n",
	"License already accepted (version %s).\n":                                                                                  "许可已接受（版本 %s）。\n",
	"License NOT accepted (need version %s).\n":                                                                                 "许可未接受（需要版本 %s）。\n",

	// models.go (embedded provider labels)
	"OpenAI":              "OpenAI",
	"Kimi (Moonshot)":     "Kimi（月之暗面）",
	"GLM (Zhipu AI)":      "GLM（智谱 AI）",
	"DeepSeek":            "DeepSeek",
	"DashScope (Alibaba)": "DashScope（阿里云）",
	"Gemini (Google)":     "Gemini（谷歌）",
	"Ollama (local)":      "Ollama（本地）",

	// models_cmd.go
	"No model configs found in configs/models.yml.":                       "configs/models.yml 中未找到模型配置。",
	"Saved model config %q. Run `mas-launcher model` to manage models.\n": "已保存模型配置 %q。运行 `mas-launcher model` 管理模型。\n",
	"%-12s provider=%-9s model=%s":                                        "%-12s 提供商=%-9s 模型=%s",
	"  (default)":                                                         "  （默认）",
	"Deleted %q. No model configs remain.\n":                              "已删除 %q。不再有任何模型配置。\n",
	"Deleted %q.\n":                                                       "已删除 %q。\n",
	"Default is now %q.\n":                                                "默认模型现为 %q。\n",
	"Default model is now %q.\n":                                          "默认模型现为 %q。\n",
	"Saved model config %q.\n":                                            "已保存模型配置 %q。\n",
	"No model configs remain. Run `mas-launcher model` to set one up.":    "已无模型配置。运行 `mas-launcher model` 进行设置。",
	"Config name required.":                                               "必须提供配置名。",
	"Config %q already exists.\n":                                         "配置 %q 已存在。\n",
	"This will be the default model.":                                     "这将作为默认模型。",
	"Note: api_key is stored in plaintext in configs/models.yml.":         "注意：api_key 以明文存储在 configs/models.yml 中。",
	"Could not fetch model list: %v\n":                                    "无法获取模型列表：%v\n",
	"Editing %q (%s / %s).\n":                                             "正在编辑 %q（%s / %s）。\n",
	"Renamed %q to %q.\n":                                                 "已将 %q 重命名为 %q。\n",
	"Existing model configs:":                                             "现有模型配置：",
	"  0) Create new model":                                               "  0) 新建模型",
	"  d) Delete a model":                                                 "  d) 删除模型",
	"  s) Set default model":                                              "  s) 设为默认模型",
	"  q) Back":                                                           "  q) 返回",
	"  q) quit/back":                                                      "  q) 退出/返回",
	"Invalid choice.":                                                     "无效选择。",
	"Custom API host":                                                     "自定义 API 地址",
	"Select provider:":                                                    "选择服务商：",
	"Manual entry / other":                                                "手动输入 / 其他",
	"Provider value (e.g. openai, azure)":                                 "服务商值（例如 openai、azure）",
	"API host (optional)":                                                 "API 地址（可选）",
	"API host (e.g. https://your-host/v1)":                                "API 地址（例如 https://your-host/v1）",
	"API key":                                                             "API 密钥",
	"Model id":                                                            "模型 ID",
	"Select model:":                                                       "选择模型：",
	"Enter model id manually":                                             "手动输入模型 ID",
	"Config name (alias)":                                                 "配置名（别名）",
	"Set as default model":                                                "设为默认模型",
	"Enable streaming":                                                    "启用流式输出",
	"Temperature":                                                         "温度",
	"Max tokens":                                                          "最大令牌数",
	"Top P":                                                               "Top P",
	"Configure advanced params":                                           "配置高级参数",
	"Multimodal":                                                          "多模态",
	"Online search":                                                       "联网搜索",
	"Enable thinking":                                                     "启用思考",
	"Thinking budget":                                                     "思考预算",
	"Top K":                                                               "Top K",
	`extra_body (JSON, e.g. {"thinking":{"type":"enabled"}})`: `extra_body（JSON，例如 {"thinking":{"type":"enabled"}}）`,
	"Input price (USD/1M tokens)":                             "输入价格（美元/百万 tokens）",
	"Output price (USD/1M tokens)":                            "输出价格（美元/百万 tokens）",
	"Cached price (USD/1M tokens)":                            "缓存价格（美元/百万 tokens）",
	"Change provider or model":                                "更换服务商或模型",
	"Config name":                                             "配置名",
	"Select a model to delete:":                               "选择要删除的模型：",
	"Cancel":                                                  "取消",
	"Select the default model:":                               "选择默认模型：",
	"Type yes to delete %q: ":                                 "输入 yes 以删除 %q：",
	"--provider and --model are required with --name":         "--name 必须搭配 --provider 和 --model",
	"model config %q not found":                               "模型配置 %q 未找到",
	"provider required":                                       "必须提供服务商",
	"API host required for a custom provider":                 "自定义服务商必须提供 API 地址",
	"model name required":                                     "必须提供模型名",
	"config %q already exists":                                "配置 %q 已存在",

	// napcat.go
	"NapCat Shell mode is not supported on this platform.":                             "此平台不支持 NapCat Shell 模式。",
	"Docker deployment: see deploy/README.md":                                          "Docker 部署：参见 deploy/README.md",
	"─ Onebot V11 (reverse WebSocket) ─":                                               "─ Onebot V11（反向 WebSocket）─",
	"  Listen:   ws://%s:8080/onebot/v11/\n":                                           "  监听：ws://%s:8080/onebot/v11/\n",
	"  The protocol implementation (e.g. NapCat) connects here as a WebSocket client.": "  协议实现（例如 NapCat）以 WebSocket 客户端身份连接到这里。",
	"─ Core ─":                       "─ Core ─",
	"  WebSocket:   ws://%s:%d/ws\n": "  WebSocket：ws://%s:%d/ws\n",
	"  IPC Secret:  %s\n":            "  IPC 密钥：%s\n",
	"Download failed: %v\n":          "下载失败：%v\n",
	"Download manually from https://github.com/NapNeko/NapCatQQ/releases":         "请从 https://github.com/NapNeko/NapCatQQ/releases 手动下载",
	"Download NapCat.Shell.zip from https://github.com/NapNeko/NapCatQQ/releases": "从 https://github.com/NapNeko/NapCatQQ/releases 下载 NapCat.Shell.zip",
	"and extract it to: %s\n":                                                          "并解压到：%s\n",
	"launcher script not found in: %s\n":                                               "在 %s 中未找到启动脚本\n",
	"Make sure NapCat.Shell is extracted and try again.":                               "请确认 NapCat.Shell 已解压后重试。",
	"warning: could not auto-configure Onebot: %v\n":                                   "警告：无法自动配置 Onebot：%v\n",
	"NapCat started in background (PID %d).\n":                                         "NapCat 已在后台启动（PID %d）。\n",
	"NapCat WebUI: %s\n":                                                               "NapCat WebUI：%s\n",
	"NapCat WebUI: http://127.0.0.1:6099/webui (token will appear after first launch)": "NapCat WebUI：http://127.0.0.1:6099/webui（首次启动后会出现 token）",
	"Open the WebUI to scan QR and log in.":                                            "打开 WebUI 扫码登录。",
	`Run the NapCat installer (requires sudo):

  curl -o napcat.sh https://nclatest.znin.net/NapNeko/NapCat-Installer/main/script/install.sh && sudo bash napcat.sh

Advanced options: --tui  --docker [y/n]  --cli [y/n]  --proxy [0-6]  --force
Docker deployment: see deploy/README.md`: `运行 NapCat 安装脚本（需要 sudo）：

  curl -o napcat.sh https://nclatest.znin.net/NapNeko/NapCat-Installer/main/script/install.sh && sudo bash napcat.sh

高级选项：--tui  --docker [y/n]  --cli [y/n]  --proxy [0-6]  --force
Docker 部署：参见 deploy/README.md`,
	`NapCat Shell mode is not available on macOS.
Refer to https://napneko.pages.dev or use Docker:
  deploy/README.md`: `macOS 上不提供 NapCat Shell 模式。
参见 https://napneko.pages.dev 或使用 Docker：
  deploy/README.md`,
	"NapCat stopped (PID %d).\n":                                    "NapCat 已停止（PID %d）。\n",
	"Downloading %s...\n":                                           "正在下载 %s...\n",
	"Extracting...":                                                 "正在解压...",
	"NapCat directory":                                              "NapCat 目录",
	"Download NapCat.Shell.zip to this directory":                   "将 NapCat.Shell.zip 下载到此目录",
	"QQ number (optional, or leave empty to scan QR in WebUI)":      "QQ 号（可选，留空则到 WebUI 扫码）",
	"failed to start NapCat: %w":                                    "启动 NapCat 失败：%w",
	"no napcat directory configured; run mas-launcher napcat first": "尚未配置 napcat 目录；请先运行 mas-launcher napcat",
	"cannot enumerate NapCat processes: %w":                         "无法枚举 NapCat 进程：%w",
	"multiple NapCat processes found (%v); stop manually":           "发现多个 NapCat 进程（%v）；请手动停止",
	"napcat process not found (is it running?)":                     "未找到 napcat 进程（它是否在运行？）",
	"not a number: %q":                                              "不是数字：%q",
	"download failed: %s":                                           "下载失败：%s",
	"GitHub API returned %s":                                        "GitHub API 返回 %s",
	"failed to parse GitHub release: %w":                            "解析 GitHub Release 失败：%w",
	"NapCat.Shell.zip not found in latest release":                  "最新 Release 中未找到 NapCat.Shell.zip",

	// repository.go
	"cannot fetch repository; use a GitHub URL or install Git": "无法拉取仓库；请使用 GitHub 地址或安装 Git",
	"repository download failed: %s":                           "仓库下载失败：%s",
	"empty repository archive":                                 "仓库压缩包为空",
	"archive contains an invalid path":                         "压缩包包含非法路径",

	// models_fetch.go
	"empty model list from %s": "%s 返回的模型列表为空",
	"%s: invalid JSON: %w":     "%s：无效 JSON：%w",
}

// T returns the translation of key in the current language, falling back to
// the key itself (English) when no translation exists. When args are given the
// translated format string is formatted with fmt.Sprintf; callers must pass a
// verb sequence matching the key.
func T(key string, args ...any) string {
	s := key
	if detectLang() == "zh" {
		if t, ok := zhMessages[key]; ok {
			s = t
		}
	}
	if len(args) == 0 {
		return s
	}
	return fmt.Sprintf(s, args...)
}

// detectLang resolves the current language with priority MUIKA_LANG >
// LC_ALL > LANG > system UI language. It returns "" when nothing is
// recognized, which lets T fall back to the English key.
func detectLang() string {
	for _, name := range []string{"MUIKA_LANG", "LC_ALL", "LANG"} {
		if s := normalizeLang(os.Getenv(name)); s != "" {
			return s
		}
	}
	return normalizeLang(systemUILang())
}

// normalizeLang maps a locale string to a supported language id (zh / en),
// stripping charset and region suffixes. Unknown locales return "".
func normalizeLang(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexAny(s, "_-"); i >= 0 {
		s = s[:i]
	}
	switch s {
	case "zh":
		return "zh"
	case "en":
		return "en"
	}
	return ""
}
