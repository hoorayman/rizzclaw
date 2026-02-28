# RizzClaw 🐾

<p align="center">
  <img src="docs/pics/logo.png" alt="RizzClaw Logo" width="200"/>
</p>

<p align="center">
  <strong>智能 AI 编程助手</strong>
</p>

<p align="center">
  <a href="https://github.com/hoorayman/rizzclaw">
    <img src="https://img.shields.io/github/license/hoorayman/rizzclaw" alt="license">
  </a>
  <a href="https://go.dev/">
    <img src="https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat&logo=go" alt="go version">
  </a>
</p>

## 简介

RizzClaw 是一个由 AI 驱动的智能编程助手，基于 [MiniMax](https://www.minimaxi.com/) 大语言模型构建。它可以帮助开发者完成代码编写、调试、文件操作、Web 搜索等多种开发任务。

## ✨ 核心特性

### 🧠 智能会话管理 (Session Auto-Summary)

RizzClaw 内置了智能的会话压缩机制，能够在长对话中自动管理上下文窗口：

- **自动压缩触发**: 当会话 token 数量超过阈值（默认 128K 的 50%）时自动触发压缩
- **智能摘要生成**: 将早期对话压缩为简洁摘要，保留关键信息的同时大幅减少 token 占用
- **可配置参数**:
  - `MaxTokens`: 最大 token 限制 (默认 128000)
  - `MaxHistoryShare`: 历史消息最大占比 (默认 0.5)
  - `MinMessagesToKeep`: 保留的最近消息数 (默认 10 条)
  - `ChunkRatio`: 压缩比例 (默认 0.4)

```
┌─────────────────────────────────────────────────────────────┐
│  会话历史                                                    │
├─────────────────────────────────────────────────────────────┤
│  [摘要] 早期 50 条消息的摘要...                              │
│  [摘要] 中期 30 条消息的摘要...                              │
├─────────────────────────────────────────────────────────────┤
│  [最近消息] 用户: 帮我重构这个函数                           │
│  [最近消息] 助手: 好的，我来帮你...                          │
│  ...                                                        │
└─────────────────────────────────────────────────────────────┘
```

### 💾 长期记忆系统 (BM25 + RAG)

RizzClaw 实现了先进的混合检索记忆系统，支持长期知识存储和智能召回：

#### 混合检索架构

```
                    ┌──────────────────┐
                    │   用户查询        │
                    └────────┬─────────┘
                             │
              ┌──────────────┼──────────────┐
              ▼              ▼              ▼
     ┌────────────────┐ ┌────────────────┐
     │  向量检索       │ │  BM25 关键词   │
     │  (语义相似)     │ │  (精确匹配)    │
     └───────┬────────┘ └───────┬────────┘
             │                  │
             │    ┌─────────────┘
             ▼    ▼
     ┌────────────────────┐
     │   分数融合 (RRF)    │
     │  Vector: 0.7       │
     │  Keyword: 0.3      │
     └─────────┬──────────┘
               ▼
     ┌────────────────────┐
     │   MMR 多样性重排    │
     │   (可选)           │
     └─────────┬──────────┘
               ▼
     ┌────────────────────┐
     │   时间衰减调整      │
     │   (可选)           │
     └─────────┬──────────┘
               ▼
         最终结果
```

#### 核心技术

| 技术 | 说明 |
|------|------|
| **BM25 全文检索** | 基于 SQLite FTS5 实现，支持中英文关键词搜索，精确匹配用户查询中的关键术语 |
| **向量语义检索** | 支持接入 Embedding 模型，实现语义级别的相似度搜索，理解查询意图而非仅匹配关键词 |
| **混合分数融合** | 向量检索权重 0.7 + 关键词检索权重 0.3，可自定义调整 |
| **MMR 多样性重排** | Maximal Marginal Relevance 算法，避免返回结果过于相似，提升信息覆盖度 |
| **时间衰减机制** | 基于指数衰减函数，越久远的记忆权重越低，半衰期默认 30 天 |
| **常青记忆 (Evergreen)** | 标记为 Evergreen 的记忆永不衰减，适合存储用户偏好、项目背景等长期有效信息 |

#### 记忆存储

```go
type MemoryEntry struct {
    ID          string    // 唯一标识
    Content     string    // 记忆内容
    Embedding   []float32 // 向量嵌入
    Keywords    []string  // 提取的关键词
    Source      string    // 来源 (如 "MEMORY.md", "conversation")
    CreatedAt   time.Time // 创建时间
    IsEvergreen bool      // 是否为常青记忆
}
```

#### 搜索配置

```go
type SearchOptions struct {
    MaxResults     int     // 最大返回数量 (默认 6)
    MinScore       float64 // 最低分数阈值 (默认 0.35)
    VectorWeight   float64 // 向量检索权重 (默认 0.7)
    KeywordWeight  float64 // 关键词检索权重 (默认 0.3)
    UseMMR         bool    // 是否启用 MMR (默认 false)
    MMRLambda      float64 // MMR 相关性权重 (默认 0.7)
    TemporalDecay  bool    // 是否启用时间衰减 (默认 true)
    HalfLifeDays   float64 // 时间衰减半衰期 (默认 30 天)
}
```

### 🛠️ 其他特性

- 🤖 **AI 驱动** - 基于 MiniMax M2.1/M2.5 等先进大语言模型
- 🧰 **丰富工具** - 内置文件操作、代码执行、Web 搜索等多种工具
- ⚡ **技能系统** - 支持加载自定义技能扩展
- 🔍 **Web 搜索** - 使用 DuckDuckGo HTML 搜索，读取环境变量自动配置代理：
  - `HTTP_PROXY` / `http_proxy` - HTTP 代理
  - `HTTPS_PROXY` / `https_proxy` - HTTPS 代理
  - `NO_PROXY` / `no_proxy` - 跳过代理的域名

## 技术栈

- **语言**: Go 1.23+
- **CLI 框架**: [Cobra](https://github.com/spf13/cobra)
- **配置管理**: [Viper](https://github.com/spf13/viper)
- **数据库**: SQLite3 (记忆存储)
- **全文检索**: SQLite FTS5 (BM25)
- **LLM 提供商**: MiniMax API

## 快速开始

### 界面预览

<p align="center">
  <img src="docs/pics/console.png" alt="RizzClaw Console" width="600"/>
</p>

### 安装

```bash
# 克隆项目
git clone https://github.com/hoorayman/rizzclaw.git
cd rizzclaw

# 构建
go build -o rizzclaw ./main.go

# 运行
./rizzclaw chat
```

### 配置

1. 复制配置文件示例：

```bash
cp config.example.json ~/.rizzclaw/config.json
```

2. 编辑 `config.json`，填入你的 MiniMax API Key：

```json
{
  "models": {
    "mode": "merge",
    "providers": {
      "minimax": {
        "baseUrl": "https://api.minimaxi.com/anthropic",
        "apiKey": "API_KEY",
        "api": "anthropic-messages",
        "models": [
          {
            "id": "MiniMax-M2.1",
            "name": "MiniMax M2.1",
            "reasoning": false,
            "input": ["text"],
            "cost": {
              "input": 0.3,
              "output": 1.2,
              "cacheRead": 0.03,
              "cacheWrite": 0.12
            },
            "contextWindow": 200000,
            "maxTokens": 8192
          },
          {
            "id": "MiniMax-M2.1-lightning",
            "name": "MiniMax M2.1 Lightning",
            "reasoning": false,
            "input": ["text"],
            "cost": {
              "input": 0.3,
              "output": 1.2,
              "cacheRead": 0.03,
              "cacheWrite": 0.12
            },
            "contextWindow": 200000,
            "maxTokens": 8192
          },
          {
            "id": "MiniMax-M2.5",
            "name": "MiniMax M2.5",
            "reasoning": true,
            "input": ["text"],
            "cost": {
              "input": 0.3,
              "output": 1.2,
              "cacheRead": 0.03,
              "cacheWrite": 0.12
            },
            "contextWindow": 200000,
            "maxTokens": 8192
          },
          {
            "id": "MiniMax-M2.5-Lightning",
            "name": "MiniMax M2.5 Lightning",
            "reasoning": true,
            "input": ["text"],
            "cost": {
              "input": 0.3,
              "output": 1.2,
              "cacheRead": 0.03,
              "cacheWrite": 0.12
            },
            "contextWindow": 200000,
            "maxTokens": 8192
          },
          {
            "id": "MiniMax-VL-01",
            "name": "MiniMax VL 01",
            "reasoning": false,
            "input": ["text", "image"],
            "cost": {
              "input": 0.3,
              "output": 1.2,
              "cacheRead": 0.03,
              "cacheWrite": 0.12
            },
            "contextWindow": 200000,
            "maxTokens": 8192
          }
        ]
      }
    }
  },
  "agents": {
    "defaults": {
      "model": {
        "minimax/MiniMax-M2.5": {
          "primary": "minimax/MiniMax-M2.5",
          "alias": "Minimax"
        }
      },
      "timeout": 120
    }
  }
}
```

### 使用

```bash
# 查看帮助
rizzclaw --help

# 启动交互式对话
rizzclaw chat

# 查看可用模型
rizzclaw models

# 查看当前配置
rizzclaw config show
```

### 交互命令

在 chat 模式下，支持以下命令：

| 命令 | 说明 |
|------|------|
| `/exit` 或 `/quit` | 退出对话 |
| `/clear` | 清除当前会话历史 |
| `/help` | 显示帮助信息 |

## 项目结构

```
rizzclaw/
├── cmd/                # CLI 命令入口
│   └── root.go         # 根命令和 chat 命令
├── internal/           # 内部包
│   ├── agent/          # Agent 核心逻辑
│   │   ├── agent.go    # Agent 实现
│   │   └── session.go  # 会话管理
│   ├── llm/            # LLM 客户端抽象
│   ├── tools/          # 工具集
│   ├── context/        # 上下文管理
│   │   ├── manager.go  # 上下文管理器
│   │   ├── session.go  # 会话存储与压缩
│   │   ├── memory.go   # 记忆存储与检索
│   │   └── types.go    # 类型定义
│   ├── minimax/        # MiniMax API 集成
│   └── config/         # 配置管理
├── docs/               # 文档资源
│   └── pics/           # 图片资源
└── main.go             # 程序入口
```

## 支持的模型

| 模型名称 | 类型 | 上下文窗口 | 最大输出 |
|---------|------|-----------|---------|
| MiniMax-M2.1 | 文本 | 200K | 8K |
| MiniMax-M2.1-lightning | 文本 | 200K | 8K |
| MiniMax-M2.5 | 推理 | 200K | 8K |
| MiniMax-M2.5-Lightning | 推理 | 200K | 8K |
| MiniMax-VL-01 | 多模态 | 200K | 8K |

## 数据存储位置

所有数据存储在用户目录下的 `.rizzclaw` 文件夹：

```
~/.rizzclaw/
├── config.json        # 配置文件
├── memory.db          # 记忆数据库 (SQLite)
├── sessions/          # 会话存储
│   └── session-*.jsonl
└── context/           # 上下文文件
    ├── MEMORY.md      # 长期记忆
    ├── USER.md        # 用户偏好
    └── ...
```

## 许可证

MIT License - 查看 [LICENSE](LICENSE) 文件了解详情

---

<p align="center">Made with ❤️ by RizzClaw</p>
