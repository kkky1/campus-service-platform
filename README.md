# Campus Service Platform

面向高校学生的校园服务平台：校园店铺查询、校园福利券抢购、学生动态分享、关注互动、签到积分。后端为 Go 实现（由 Spring Boot 版本重写而来，对外行为等价），前端为静态资源（Vue + Element UI + axios），经 Nginx 反向代理访问后端。

## 技术栈

- 后端：Go 1.24、Gin、GORM、go-redis、segmentio/kafka-go、slog
- 存储与中间件：MySQL 8、Redis（6.2+）、Kafka（KRaft 单节点即可）
- 前端：Vue、Element UI、Axios、Nginx 静态资源服务
- 部署：Docker Compose、Nginx 反向代理、多阶段构建（~20MB 镜像）

## 核心功能

- 手机验证码登录：Redis 存储登录 Token（Hash，30 分钟），双中间件（Token 刷新 / 登录校验），401 语义不变。
- 校园服务查询：店铺类型列表（Redis List 缓存）、店铺详情（逻辑过期缓存 + 互斥锁异步重建）、附近店铺（Redis GEO 5000m 排序分页）。
- 校园福利券抢购：Redis + Lua 原子完成库存校验、防重复抢券、扣减库存。
- 异步下单削峰：抢券资格校验通过后发布 Kafka 消息，消费者幂等建单（`INSERT ... ON DUPLICATE KEY`）并条件扣库存；失败重试 3 次（指数退避）后进入 DLT 死信主题由兜底消费者处理。
- 社交动态：发布动态推送粉丝 feed（ZSET）、滚动分页、点赞切换、热门榜、关注与共同关注。
- 签到积分：Redis BITFIELD 位图签到与连续天数统计。
- 图片上传：hash 目录存储，路径格式与旧版一致（可配置上传根目录）。

## 登录说明（临时）

当前为方便体验，**登录不校验短信验证码**（`config.yaml` 中 `login.skip_code: true`，环境变量 `LOGIN_SKIP_CODE` 可覆盖）：
登录页只需填手机号、验证码可留空，任意手机号首次登录会自动注册。「发送验证码」按钮仍可用（验证码会写入 Redis 但不再校验）。

恢复校验：把 `config.yaml` 的 `login.skip_code` 改为 `false`（或设 `LOGIN_SKIP_CODE=false`）后重建部署即可，后端逻辑与前端校验开关均已保留。

## RAG 知识库（可选模块）

把非结构化文档变成可检索、可引用、可评估的校园知识层。核心链路：

```
文档(txt/md/csv/json/html/docx/xlsx/pptx/pdf)
   → 解析(Block) → 模板化切分 → 知识编译(标题路径/标签/摘要) → 向量化 → 落库
提问 → 混合检索(BM25 + 向量余弦, 7:3 融合) → 上下文组装 → LLM 生成 → 逐句引用对齐 [ID:n]
```

### 能力清单

| 环节 | 实现 |
|---|---|
| 多文档解析 | txt/md（标题表格列表）、csv、json（路径展平）、html、docx（标题样式/表格）、xlsx（多 Sheet/共享字符串）、pptx（分页）、pdf（纯 Go 文本提取） |
| 切分模板 | `naive`（标题分节+重叠）、`qa`（问答对）、`table`（整表保留/按行拆）、`resume`（列表项独立）、`one`（整篇）、paper/book/manual/laws/presentation（按标题分节） |
| 知识编译 | 文档标签（停用词过滤+词频）、摘要、切片标题路径 |
| Embedding | 本地特征哈希（离线确定性，默认 384 维）或任意 OpenAI 兼容 `/v1/embeddings` |
| 混合检索 | BM25(k1=1.5,b=0.75) + 向量余弦，权重可配，标题/标签命中加成 |
| 生成引用 | OpenAI 兼容 LLM（默认 DeepSeek），逐句余弦对齐（阈值 0.63 起步 0.8 衰减），代码块保护，每句最多 4 条 |
| 评估 | RAGAS 精简：faithfulness / answer_relevancy / context_precision / context_recall + **自定义 citation_accuracy（引用准确率）** |

### HTTP API（均需登录）

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/rag/kb` | 创建知识库 `{name, description}` |
| GET | `/rag/kb/list` | 知识库列表 |
| DELETE | `/rag/kb/:id` | 删除知识库（级联） |
| POST | `/rag/doc/upload` | 上传文档（multipart：`kbId`、`chunkMethod`、`file`），异步入库 |
| GET | `/rag/doc/list?kbId=` | 文档列表（含 status/chunkCount/tags/summary） |
| GET | `/rag/doc/:id/chunks` | 文档切片 |
| DELETE | `/rag/doc/:id` | 删除文档 |
| POST | `/rag/retrieve` | 混合检索调试 `{kbId, question, topK}` |
| POST | `/rag/chat` | 问答 `{kbId, question, topK}` → `{answer, references[], cited[], latencyMs}` |
| POST | `/rag/eval/run` | 评估 `{kbId, name, usePipeline, cases:[{question, groundTruth, answer, contexts}]}` |
| GET | `/rag/eval/:id` | 评估结果（逐样本指标 + 聚合） |

### 配置（config.yaml `rag:` 段，全部支持环境变量覆盖）

```yaml
rag:
  chunk: { token_num: 256, overlap_percent: 10 }
  embedding: { provider: local, dim: 384 }     # provider: local | openai (+base_url/api_key/model)
  llm: { base_url: https://api.deepseek.com, model: deepseek-chat }  # key 用 RAG_LLM_API_KEY
  retrieval: { top_k: 8, vector_weight: 0.7, keyword_weight: 0.3 }
  citation: { min_sentence_len: 5, threshold: 0.63 }
```

环境变量：`RAG_EMBEDDING_PROVIDER`、`RAG_EMBEDDING_BASE_URL/API_KEY/MODEL/DIM`、`RAG_LLM_BASE_URL/API_KEY/MODEL`。
Docker 部署时把密钥写入仓库根目录 `.env`（已在 .gitignore 中）：

```
RAG_LLM_API_KEY=sk-...
```

### 快速体验

```bash
TOKEN=$(curl -s -X POST 'localhost:8081/user/login' -H 'Content-Type: application/json' \
  -d '{"phone":"13800000000","code":"<redis 中的验证码>"}' | jq -r .data)
KB=$(curl -s -X POST localhost:8081/rag/kb -H "authorization: $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"校园知识库"}' | jq -r .data.id)
curl -s -X POST localhost:8081/rag/doc/upload -H "authorization: $TOKEN" \
  -F kbId=$KB -F chunkMethod=naive -F file=@校园服务手册.md
curl -s -X POST localhost:8081/rag/chat -H "authorization: $TOKEN" -H 'Content-Type: application/json' \
  -d "{\"kbId\":$KB,\"question\":\"食堂几点开门？\"}"
```

## 本地构建与运行

前置：Go 1.26+。

```bash
go build -o bin/campus-server ./cmd/server
./bin/campus-server            # 默认读取 ./config.yaml
```

配置：`config.yaml`，支持环境变量覆盖：`SERVER_PORT`、`MYSQL_DSN`、`REDIS_ADDR`、`REDIS_PASSWORD`、`KAFKA_BROKERS`、`UPLOAD_DIR`、`CONFIG_PATH`。

## Docker Compose 一键起全套

```bash
# 服务器部署：先在宿主机编译二进制（容器内下载 Go 依赖过慢），再起容器
go build -o bin/campus-server ./cmd/server
docker compose -f docker-compose.local.yml up -d --build
```

- MySQL：初始化脚本 `db/hmdp.sql`（含零日期，需放宽 sql_mode，compose 已配置；导入命令：
  `(echo "SET SESSION sql_mode='NO_ENGINE_SUBSTITUTION';"; cat db/hmdp.sql) | docker exec -i hmdp-mysql mysql -uroot -p123456 hmdp`）
- Redis：`:6379`
- Kafka：`:9092`（KRaft 单节点，主题自动创建）
- 后端：`:8081`（镜像用 `Dockerfile.prebuilt` 瘦镜像；`Dockerfile` 多阶段构建适合网络良好的 CI 环境）
- 前端 + Nginx：`:8080`（前端镜像由 `Dockerfile.front` 打包：静态资源 + nginx 配置一体；`/api` 反向代理到后端）

### 前端页面

| 页面 | 功能 |
|---|---|
| index.html | 首页：签到（+5 积分/连续天数）、按名称搜索、分类入口、动态列表 |
| shop-list.html | 店铺列表：分类切换、距离/热度/评分排序、名称搜索、触底分页 |
| shop-detail.html | 店铺详情：福利券列表、立即抢券 |
| blog-edit / blog-detail | 动态发布（关联服务点/图片上传）、详情（点赞/关注） |
| info / info-edit | 我的：签到、知识库入口、资料编辑（昵称/头像/介绍/性别/校区/生日） |
| rag.html | **知识库问答**：知识库管理、文档上传与状态、引用角标问答、RAGAS 评估面板 |

### 安全与密钥（重要）

- 所有端口仅绑定 `127.0.0.1`，只有 nginx 的 8080 对外；数据库/缓存/消息队列不再暴露。
- 密钥集中在服务器端 `.env`（**已 gitignore，不入库**）：
  `MYSQL_ROOT_PASSWORD`、`MYSQL_APP_PASSWORD`（应用专用最小权限账号）、`REDIS_PASSWORD`（Redis requirepass）、`RAG_LLM_API_KEY`。
- 2026-09-26 曾发生弱密码导致的入侵清库事件（详见 `docs/security-incident.md`），当前已按照该文档完成加固。

### 部署与访问

- **拉取式 CD**：服务器 systemd `campus-deploy.timer` 每 2 分钟执行 `scripts/deploy-poll.sh`——
  只允许 fast-forward，本地有未推送提交或未提交改动时自动跳过；有新提交则编译 → compose 重建 → 健康检查。
- **访问前端**：
  - 公网：在 VPS 服务商的 NAT 端口映射面板中，把某个公网端口映射到本机 `8080`（当前公网 80/8000 不是本机服务）。
  - 临时访问（无需映射）：`ssh -p 64540 -L 8080:127.0.0.1:8080 root@<服务器IP>`，然后打开 `http://localhost:8080`。
- **前端打包 CI**：`.github/workflows/frontend.yml` 在 front/ 变更时执行：静态资源校验 → `tar.gz + sha256`
  产物上传（Actions Artifacts）→ 构建 `Dockerfile.front` 镜像验证；部署由拉取式 CD 自动完成。

## 测试

```bash
go test ./...                 # 单元 + 契约测试（内存 Redis/SQLite，无需外部依赖）
go test ./internal/service/   # 含集成测试：需要本地 Redis 127.0.0.1:6379 与 MySQL hmdp 库，不可用时自动跳过
python3 scripts/contract_check.py   # HTTP 契约基线：37 项断言（需服务已启动；登录分支需预置验证码，见脚本注释）
```

## 目录结构

```
cmd/server/          入口装配
internal/config/     配置加载
internal/httpapi/    路由与处理器（契约测试）
internal/middleware/ 认证双中间件 + 异常恢复
internal/service/    业务层（登录/店铺/动态/关注/秒杀/上传）
internal/repo/       GORM 模型（11 张表）与自定义 SQL
internal/mq/         Kafka 生产者 + 主消费者 + DLT 兜底消费者
internal/pkg/dto/    响应契约（JSON null 省略、时间格式）
internal/pkg/rds/    Redis 键与 TTL 常量
internal/rag/        RAG 模块（parser/chunker/embed/retrieval/llm/chat/eval/ingest/store）
internal/pkg/cache/  （预留）
scripts/lua/         内嵌 Lua 脚本
db/                  hmdp.sql 初始化脚本
front/               前端静态资源（不变）
openspec/            OpenSpec 规格与变更
```

## OpenSpec

项目使用 OpenSpec 进行规格驱动开发。当前变更：`openspec/changes/rewrite-backend-in-go`（Spring Boot → Go 重写），其中 `proposal.md` / `design.md` / `specs/` 记录了行为契约与全部技术决策。
