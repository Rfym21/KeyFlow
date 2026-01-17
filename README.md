# GPT-Load

[![Release](https://img.shields.io/github/v/release/tbphp/gpt-load)](https://github.com/tbphp/gpt-load/releases)
![Go Version](https://img.shields.io/badge/Go-1.23+-blue.svg)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

一款高性能、企业级的 AI API 透明代理服务，专为需要集成多种 AI 服务的企业和开发者设计。基于 Go 语言构建，具备智能密钥管理、负载均衡和全面的监控功能，适用于高并发生产环境。

详细文档请访问 [官方文档](https://www.gpt-load.com/docs?lang=zh)

<a href="https://trendshift.io/repositories/14880" target="_blank"><img src="https://trendshift.io/api/badge/repositories/14880" alt="tbphp%2Fgpt-load | Trendshift" style="width: 250px; height: 55px;" width="250" height="55"/></a>
<a href="https://hellogithub.com/repository/tbphp/gpt-load" target="_blank"><img src="https://api.hellogithub.com/v1/widgets/recommend.svg?rid=554dc4c46eb14092b9b0c56f1eb9021c&claim_uid=Qlh8vzrWJ0HCneG" alt="Featured｜HelloGitHub" style="width: 250px; height: 54px;" width="250" height="54" /></a>

## 功能特性

- **透明代理**：完整保留原生 API 格式，支持 OpenAI、Google Gemini、Anthropic Claude 等格式
- **智能密钥管理**：高性能密钥池，支持分组管理、自动轮换和故障恢复
- **负载均衡**：多上游端点加权负载均衡，提升服务可用性
- **智能故障处理**：自动密钥黑名单管理和恢复机制，确保服务连续性
- **动态配置**：系统设置和分组配置支持热加载，无需重启
- **企业级架构**：分布式主从部署，支持水平扩展和高可用
- **现代化管理**：基于 Vue 3 的 Web 管理界面，直观易用
- **全面监控**：实时统计、健康检查和详细的请求日志
- **高性能设计**：零拷贝流式传输、连接池复用、原子操作
- **生产就绪**：优雅关闭、错误恢复和全面的安全机制
- **双重认证**：管理端和代理端分离认证，代理认证支持全局和分组级密钥

## 支持的 AI 服务

GPT-Load 作为透明代理服务，完整保留各 AI 服务商的原生 API 格式：

- **OpenAI 格式**：官方 OpenAI API、Azure OpenAI 及其他 OpenAI 兼容服务
- **Google Gemini 格式**：Gemini Pro、Gemini Pro Vision 等模型的原生 API
- **Anthropic Claude 格式**：Claude 系列模型，支持高质量对话和文本生成

## 快速开始

### 系统要求

- Go 1.23+（源码构建）
- Docker（容器化部署）
- MySQL、PostgreSQL 或 SQLite（数据库存储）
- Redis（缓存和分布式协调，可选）

### 方式一：Docker 快速启动

```bash
docker run -d --name gpt-load \
    -p 3001:3001 \
    -e AUTH_KEY=your-secure-key-here \
    -v "$(pwd)/data":/app/data \
    ghcr.io/tbphp/gpt-load:latest
```

> 请将 `your-secure-key-here` 更改为强密码（切勿使用默认值），然后可以登录管理界面：<http://localhost:3001>

### 方式二：使用 Docker Compose（推荐）

**安装命令：**

```bash
# 创建目录
mkdir -p gpt-load && cd gpt-load

# 下载配置文件
wget https://raw.githubusercontent.com/tbphp/gpt-load/refs/heads/main/docker-compose.yml
wget -O .env https://raw.githubusercontent.com/tbphp/gpt-load/refs/heads/main/.env.example

# 编辑 .env 文件，将 AUTH_KEY 更改为强密码。切勿使用默认或简单的密钥如 sk-123456。

# 启动服务
docker compose up -d
```

部署前必须更改默认管理密钥（AUTH_KEY）。推荐格式：sk-prod-[32位随机字符串]。

默认安装使用 SQLite 版本，适用于轻量级单实例应用。

如需安装 MySQL、PostgreSQL 和 Redis，请取消 `docker-compose.yml` 文件中相应服务的注释，配置对应的环境变量，然后重启。

**其他命令：**

```bash
# 检查服务状态
docker compose ps

# 查看日志
docker compose logs -f

# 重启服务
docker compose down && docker compose up -d

# 更新到最新版本
docker compose pull && docker compose down && docker compose up -d
```

部署完成后：

- 访问 Web 管理界面：<http://localhost:3001>
- API 代理地址：<http://localhost:3001/proxy>

> 使用修改后的 AUTH_KEY 登录管理界面。

### 方式三：源码构建

源码构建需要本地安装数据库（SQLite、MySQL 或 PostgreSQL）和 Redis（可选）。

```bash
# 克隆并构建
git clone https://github.com/tbphp/gpt-load.git
cd gpt-load
go mod tidy

# 创建配置
cp .env.example .env

# 编辑 .env 文件，将 AUTH_KEY 更改为强密码。切勿使用默认或简单的密钥如 sk-123456。
# 修改 .env 中的 DATABASE_DSN 和 REDIS_DSN 配置
# REDIS_DSN 为可选项；如不配置，将启用内存存储

# 运行
make run
```

部署完成后：

- 访问 Web 管理界面：<http://localhost:3001>
- API 代理地址：<http://localhost:3001/proxy>

> 使用修改后的 AUTH_KEY 登录管理界面。

### 方式四：集群部署

集群部署要求所有节点连接相同的 MySQL（或 PostgreSQL）和 Redis，Redis 为必需项。建议使用统一的分布式 MySQL 和 Redis 集群。

**部署要求：**

- 所有节点必须配置相同的 `AUTH_KEY`、`DATABASE_DSN`、`REDIS_DSN`
- 主从架构中，从节点必须配置环境变量：`IS_SLAVE=true`

详情请参阅 [集群部署文档](https://www.gpt-load.com/docs/cluster?lang=zh)

## 配置系统

### 配置架构概述

GPT-Load 采用双层配置架构：

#### 1. 静态配置（环境变量）

- **特性**：应用启动时读取，运行时不可变，需重启应用生效
- **用途**：基础设施配置，如数据库连接、服务器端口、认证密钥等
- **管理**：通过 `.env` 文件或系统环境变量设置

#### 2. 动态配置（热加载）

- **系统设置**：存储在数据库中，为整个应用提供统一的行为标准
- **分组配置**：为特定分组定制的行为参数，可覆盖系统设置
- **配置优先级**：分组配置 > 系统设置 > 环境配置
- **特性**：支持热加载，修改后立即生效，无需重启应用

<details>
<summary>静态配置（环境变量）</summary>

**服务器配置：**

| 设置项           | 环境变量                           | 默认值          | 说明                                   |
| ---------------- | ---------------------------------- | --------------- | -------------------------------------- |
| 服务端口         | `PORT`                             | 3001            | HTTP 服务器监听端口                    |
| 服务地址         | `HOST`                             | 0.0.0.0         | HTTP 服务器绑定地址                    |
| 读取超时         | `SERVER_READ_TIMEOUT`              | 60              | HTTP 服务器读取超时（秒）              |
| 写入超时         | `SERVER_WRITE_TIMEOUT`             | 600             | HTTP 服务器写入超时（秒）              |
| 空闲超时         | `SERVER_IDLE_TIMEOUT`              | 120             | HTTP 连接空闲超时（秒）                |
| 优雅关闭超时     | `SERVER_GRACEFUL_SHUTDOWN_TIMEOUT` | 10              | 服务优雅关闭等待时间（秒）             |
| 从节点模式       | `IS_SLAVE`                         | false           | 集群部署的从节点标识                   |
| 时区             | `TZ`                               | `Asia/Shanghai` | 指定时区                               |

**安全配置：**

| 设置项     | 环境变量         | 默认值 | 说明                                                         |
| ---------- | ---------------- | ------ | ------------------------------------------------------------ |
| 管理密钥   | `AUTH_KEY`       | -      | **管理端**的访问认证密钥，请更改为强密码                     |
| 加密密钥   | `ENCRYPTION_KEY` | -      | 加密存储的 API 密钥。支持任意字符串，留空则禁用加密。参见 [数据加密迁移](#数据加密迁移) |

**数据库配置：**

| 设置项       | 环境变量       | 默认值               | 说明                                       |
| ------------ | -------------- | -------------------- | ------------------------------------------ |
| 数据库连接   | `DATABASE_DSN` | `./data/gpt-load.db` | 数据库连接字符串（DSN）或文件路径          |
| Redis 连接   | `REDIS_DSN`    | -                    | Redis 连接字符串，为空时使用内存存储       |

**性能与 CORS 配置：**

| 设置项           | 环境变量                  | 默认值                        | 说明                       |
| ---------------- | ------------------------- | ----------------------------- | -------------------------- |
| 最大并发请求数   | `MAX_CONCURRENT_REQUESTS` | 100                           | 系统允许的最大并发请求数   |
| 启用 CORS        | `ENABLE_CORS`             | false                         | 是否启用跨域资源共享       |
| 允许的源         | `ALLOWED_ORIGINS`         | -                             | 允许的源，逗号分隔         |
| 允许的方法       | `ALLOWED_METHODS`         | `GET,POST,PUT,DELETE,OPTIONS` | 允许的 HTTP 方法           |
| 允许的请求头     | `ALLOWED_HEADERS`         | `*`                           | 允许的请求头，逗号分隔     |
| 允许凭据         | `ALLOW_CREDENTIALS`       | false                         | 是否允许发送凭据           |

**日志配置：**

| 设置项         | 环境变量          | 默认值                | 说明                              |
| -------------- | ----------------- | --------------------- | --------------------------------- |
| 日志级别       | `LOG_LEVEL`       | `info`                | 日志级别：debug、info、warn、error |
| 日志格式       | `LOG_FORMAT`      | `text`                | 日志格式：text、json              |
| 启用文件日志   | `LOG_ENABLE_FILE` | false                 | 是否启用文件日志输出              |
| 日志文件路径   | `LOG_FILE_PATH`   | `./data/logs/app.log` | 日志文件存储路径                  |

**代理配置：**

GPT-Load 自动读取环境变量中的代理设置，用于向上游 AI 服务商发起请求。

| 设置项      | 环境变量       | 默认值 | 说明                               |
| ----------- | -------------- | ------ | ---------------------------------- |
| HTTP 代理   | `HTTP_PROXY`   | -      | HTTP 请求的代理服务器地址          |
| HTTPS 代理  | `HTTPS_PROXY`  | -      | HTTPS 请求的代理服务器地址         |
| 代理排除    | `NO_PROXY`     | -      | 绕过代理的主机或域名列表，逗号分隔 |

支持的代理协议格式：

- **HTTP**：`http://user:pass@host:port`
- **HTTPS**：`https://user:pass@host:port`
- **SOCKS5**：`socks5://user:pass@host:port`
</details>

<details>
<summary>动态配置（热加载）</summary>

**基础设置：**

| 设置项             | 字段名                               | 默认值                  | 分组覆盖 | 说明                           |
| ------------------ | ------------------------------------ | ----------------------- | -------- | ------------------------------ |
| 项目 URL           | `app_url`                            | `http://localhost:3001` | ❌       | 项目基础 URL                   |
| 全局代理密钥       | `proxy_keys`                         | 初始值取自 `AUTH_KEY`   | ❌       | 全局生效的代理密钥，逗号分隔   |
| 日志保留天数       | `request_log_retention_days`         | 7                       | ❌       | 请求日志保留天数，0 表示不清理 |
| 日志写入间隔       | `request_log_write_interval_minutes` | 1                       | ❌       | 日志写入数据库周期（分钟）     |
| 启用请求体日志     | `enable_request_body_logging`        | false                   | ✅       | 是否在请求日志中记录完整的请求体内容 |

**请求设置：**

| 设置项               | 字段名                    | 默认值 | 分组覆盖 | 说明                                     |
| -------------------- | ------------------------- | ------ | -------- | ---------------------------------------- |
| 请求超时             | `request_timeout`         | 600    | ✅       | 转发请求完整生命周期超时（秒）           |
| 连接超时             | `connect_timeout`         | 15     | ✅       | 与上游服务建立连接的超时（秒）           |
| 空闲连接超时         | `idle_conn_timeout`       | 120    | ✅       | HTTP 客户端空闲连接超时（秒）            |
| 响应头超时           | `response_header_timeout` | 600    | ✅       | 等待上游响应头的超时（秒）               |
| 最大空闲连接数       | `max_idle_conns`          | 100    | ✅       | 连接池最大空闲连接总数                   |
| 每主机最大空闲连接数 | `max_idle_conns_per_host` | 50     | ✅       | 每个上游主机的最大空闲连接数             |
| 代理 URL             | `proxy_url`               | -      | ✅       | 转发请求的 HTTP/HTTPS 代理，为空则使用环境配置 |

**密钥配置：**

| 设置项           | 字段名                            | 默认值 | 分组覆盖 | 说明                                             |
| ---------------- | --------------------------------- | ------ | -------- | ------------------------------------------------ |
| 最大重试次数     | `max_retries`                     | 3      | ✅       | 单次请求使用不同密钥的最大重试次数               |
| 黑名单阈值       | `blacklist_threshold`             | 3      | ✅       | 连续失败多少次后密钥进入黑名单                   |
| 密钥验证间隔     | `key_validation_interval_minutes` | 60     | ✅       | 后台定时密钥验证周期（分钟）                     |
| 密钥验证并发数   | `key_validation_concurrency`      | 10     | ✅       | 后台验证无效密钥的并发数                         |
| 密钥验证超时     | `key_validation_timeout_seconds`  | 20     | ✅       | 后台验证单个密钥的 API 请求超时（秒）            |

</details>

## 数据加密迁移

GPT-Load 支持加密存储 API 密钥。您可以随时启用、禁用或更改加密密钥。

<details>
<summary>查看数据加密迁移详情</summary>

### 迁移场景

- **启用加密**：加密明文数据存储 - 使用 `--to <新密钥>`
- **禁用加密**：将加密数据解密为明文 - 使用 `--from <当前密钥>`
- **更换加密密钥**：替换加密密钥 - 使用 `--from <当前密钥> --to <新密钥>`

### 操作步骤

#### Docker Compose 部署

```bash
# 1. 更新镜像（确保使用最新版本）
docker compose pull

# 2. 停止服务
docker compose down

# 3. 备份数据库（强烈推荐）
# 迁移前必须手动备份数据库或导出密钥，以避免因操作或异常导致密钥丢失。

# 4. 执行迁移命令
# 启用加密（your-32-char-secret-key 是您的密钥，建议使用 32 位以上的随机字符串）
docker compose run --rm gpt-load migrate-keys --to "your-32-char-secret-key"

# 禁用加密
docker compose run --rm gpt-load migrate-keys --from "your-current-key"

# 更换加密密钥
docker compose run --rm gpt-load migrate-keys --from "old-key" --to "new-32-char-secret-key"

# 5. 更新配置文件
# 编辑 .env 文件，设置 ENCRYPTION_KEY 与 --to 参数一致
# 如禁用加密，删除 ENCRYPTION_KEY 或设为空
vim .env
# 添加或修改：ENCRYPTION_KEY=your-32-char-secret-key

# 6. 重启服务
docker compose up -d
```

#### 源码构建部署

```bash
# 1. 停止服务
# 停止运行中的服务进程（Ctrl+C 或 kill 进程）

# 2. 备份数据库（强烈推荐）
# 迁移前必须手动备份数据库或导出密钥，以避免因操作或异常导致密钥丢失。

# 3. 执行迁移命令
# 启用加密
make migrate-keys ARGS="--to your-32-char-secret-key"

# 禁用加密
make migrate-keys ARGS="--from your-current-key"

# 更换加密密钥
make migrate-keys ARGS="--from old-key --to new-32-char-secret-key"

# 4. 更新配置文件
# 编辑 .env 文件，设置 ENCRYPTION_KEY 与 --to 参数一致
echo "ENCRYPTION_KEY=your-32-char-secret-key" >> .env

# 5. 重启服务
make run
```

### 重要注意事项

⚠️ **重要提醒**：
- **一旦 ENCRYPTION_KEY 丢失，加密数据将无法恢复！** 请妥善备份此密钥。建议使用密码管理器或安全的密钥管理系统
- 迁移前**必须停止服务**，以避免数据不一致
- 强烈建议**备份数据库**，以便迁移失败时恢复
- 密钥应使用 **32 位或更长的随机字符串**以确保安全
- 确保迁移后 `.env` 中的 `ENCRYPTION_KEY` 与 `--to` 参数一致
- 如禁用加密，删除或清空 `ENCRYPTION_KEY` 配置

### 密钥生成示例

```bash
# 生成安全的随机密钥（32 位）
openssl rand -base64 32 | tr -d "=+/" | cut -c1-32
```

</details>

## Web 管理界面

访问管理控制台：<http://localhost:3001>（默认地址）

Web 管理界面提供以下功能：

- **仪表盘**：实时统计和系统状态概览
- **密钥管理**：创建和配置 AI 服务商分组，添加、删除和监控 API 密钥
- **请求日志**：详细的请求历史和调试信息
- **系统设置**：全局配置管理和热加载

## API 使用指南

<details>
<summary>代理接口调用</summary>

GPT-Load 通过分组名称路由请求到不同的 AI 服务。使用方法如下：

### 1. 代理端点格式

```text
http://localhost:3001/proxy/{分组名称}/{原始API路径}
```

- `{分组名称}`：在管理界面创建的分组名称
- `{原始API路径}`：与原始 AI 服务路径完全一致

### 2. 认证方式

在 Web 管理界面配置**代理密钥**，支持系统级和分组级代理密钥。

- **认证方式**：与原生 API 一致，但将原始密钥替换为配置的代理密钥。
- **密钥范围**：系统设置中配置的**全局代理密钥**可在所有分组中使用。分组中配置的**分组代理密钥**仅对当前分组有效。
- **格式**：多个密钥用逗号分隔。

### 3. OpenAI 接口示例

假设创建了名为 `openai` 的分组：

**原始调用：**

```bash
curl -X POST https://api.openai.com/v1/chat/completions \
  -H "Authorization: Bearer sk-your-openai-key" \
  -H "Content-Type: application/json" \
  -d '{"model": "gpt-4.1-mini", "messages": [{"role": "user", "content": "Hello"}]}'
```

**代理调用：**

```bash
curl -X POST http://localhost:3001/proxy/openai/v1/chat/completions \
  -H "Authorization: Bearer your-proxy-key" \
  -H "Content-Type: application/json" \
  -d '{"model": "gpt-4.1-mini", "messages": [{"role": "user", "content": "Hello"}]}'
```

**需要更改的内容：**

- 将 `https://api.openai.com` 替换为 `http://localhost:3001/proxy/openai`
- 将原始 API Key 替换为**代理密钥**

### 4. Gemini 接口示例

假设创建了名为 `gemini` 的分组：

**原始调用：**

```bash
curl -X POST https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=your-gemini-key \
  -H "Content-Type: application/json" \
  -d '{"contents": [{"parts": [{"text": "Hello"}]}]}'
```

**代理调用：**

```bash
curl -X POST http://localhost:3001/proxy/gemini/v1beta/models/gemini-2.5-pro:generateContent?key=your-proxy-key \
  -H "Content-Type: application/json" \
  -d '{"contents": [{"parts": [{"text": "Hello"}]}]}'
```

**需要更改的内容：**

- 将 `https://generativelanguage.googleapis.com` 替换为 `http://localhost:3001/proxy/gemini`
- 将 URL 参数中的 `key=your-gemini-key` 替换为**代理密钥**

### 5. Anthropic 接口示例

假设创建了名为 `anthropic` 的分组：

**原始调用：**

```bash
curl -X POST https://api.anthropic.com/v1/messages \
  -H "x-api-key: sk-ant-api03-your-anthropic-key" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{"model": "claude-sonnet-4-20250514", "messages": [{"role": "user", "content": "Hello"}]}'
```

**代理调用：**

```bash
curl -X POST http://localhost:3001/proxy/anthropic/v1/messages \
  -H "x-api-key: your-proxy-key" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{"model": "claude-sonnet-4-20250514", "messages": [{"role": "user", "content": "Hello"}]}'
```

**需要更改的内容：**

- 将 `https://api.anthropic.com` 替换为 `http://localhost:3001/proxy/anthropic`
- 将 `x-api-key` 请求头中的原始 API Key 替换为**代理密钥**

### 6. 支持的接口

**OpenAI 格式：**

- `/v1/chat/completions` - 聊天对话
- `/v1/completions` - 文本补全
- `/v1/embeddings` - 文本嵌入
- `/v1/models` - 模型列表
- 以及所有其他 OpenAI 兼容接口

**Gemini 格式：**

- `/v1beta/models/*/generateContent` - 内容生成
- `/v1beta/models` - 模型列表
- 以及所有其他 Gemini 原生接口

**Anthropic 格式：**

- `/v1/messages` - 消息对话
- `/v1/models` - 模型列表（如可用）
- 以及所有其他 Anthropic 原生接口

### 7. 客户端 SDK 配置

**OpenAI Python SDK：**

```python
from openai import OpenAI

client = OpenAI(
    api_key="your-proxy-key",  # 使用代理密钥
    base_url="http://localhost:3001/proxy/openai"  # 使用代理端点
)

response = client.chat.completions.create(
    model="gpt-4.1-mini",
    messages=[{"role": "user", "content": "Hello"}]
)
```

**Google Gemini SDK（Python）：**

```python
import google.generativeai as genai

# 配置 API 密钥和基础 URL
genai.configure(
    api_key="your-proxy-key",  # 使用代理密钥
    client_options={"api_endpoint": "http://localhost:3001/proxy/gemini"}
)

model = genai.GenerativeModel('gemini-2.5-pro')
response = model.generate_content("Hello")
```

**Anthropic SDK（Python）：**

```python
from anthropic import Anthropic

client = Anthropic(
    api_key="your-proxy-key",  # 使用代理密钥
    base_url="http://localhost:3001/proxy/anthropic"  # 使用代理端点
)

response = client.messages.create(
    model="claude-sonnet-4-20250514",
    messages=[{"role": "user", "content": "Hello"}]
)
```

> **重要说明**：作为透明代理服务，GPT-Load 完整保留各 AI 服务的原生 API 格式和认证方式。您只需替换端点地址并使用管理界面中配置的**代理密钥**即可无缝迁移。

</details>

## 相关项目

- **[New API](https://github.com/QuantumNous/new-api)** - 优秀的 AI 模型聚合管理和分发系统

## 贡献

感谢所有为 GPT-Load 做出贡献的开发者！

[![Contributors](https://contrib.rocks/image?repo=tbphp/gpt-load)](https://github.com/tbphp/gpt-load/graphs/contributors)

## 许可证

MIT 许可证 - 详见 [LICENSE](LICENSE) 文件。

## Star 历史

[![Stargazers over time](https://starchart.cc/tbphp/gpt-load.svg?variant=adaptive)](https://starchart.cc/tbphp/gpt-load)
