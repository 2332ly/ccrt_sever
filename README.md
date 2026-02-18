# ccrt_sever

Go + Gin 的后端服务，提供用户注册/登录、短信验证码、JWT 认证、MMSE 评估/AI 评价、康复训练小游戏记录与难度推荐，以及“吃药提醒”CRUD 接口。数据库使用 MySQL，GORM 自动迁移表结构。

## 主要功能
- 用户注册/登录（支持密码或短信验证码）
- 短信验证码发送与校验
- JWT 认证中间件
- 支持 access/refresh token 刷新接口
- 吃药提醒管理（创建、查询、更新、删除）
- MMSE 量表、评估提交与 AI 评价
- 康复游戏结果记录（含答案/难度/准确率）与难度推荐
- AI 成语接口（舒尔特方格 AI 模式）

## 技术栈
- Go 1.24
- Gin
- GORM + MySQL
- JWT
- Viper

## 目录结构
- `main.go`: 服务入口
- `router/router.go`: 路由注册
- `config/`: 配置与数据库初始化
- `controllers/`: 控制器（认证、短信、用户、吃药提醒）
- `models/`: 数据模型
- `utils/`: JWT、响应、短信与中间件工具

## 配置
配置文件：`config/config.yml`

关键配置项：
- `app.port`: 服务端口（如 `:3000`）
- `database.dsn`: MySQL DSN
- `jwt.secret`: JWT 签名密钥
- `sms.*`: 短信服务配置
- `openai.*`: AI 服务配置（模型/地址/Key 等）

安全提示：`config/config.yml` 中包含密钥/账号信息，请在实际部署时改用环境变量，并避免提交到仓库。

## 启动
1. 确保 MySQL 可访问，并配置好 `database.dsn`
2. 设置 `jwt.secret`
3. 启动服务

```bash
go run .
```

## API 概览
完整接口见 `API.md`。主要接口：
- `POST /api/auth/register`
- `POST /api/auth/login`
- `POST /api/auth/refresh`
- `POST /api/auth/sms/send`
- `POST /api/auth/sms/verify`
- `GET /api/me`
- `GET /api/mmse/scale`
- `POST /api/mmse/assessments`
- `GET /api/mmse/assessments/:id`
- `POST /api/mmse/assessments/:id/ai`
- `GET /api/medications`
- `POST /api/medications`
- `GET /api/medications/:id`
- `PUT /api/medications/:id`
- `DELETE /api/medications/:id`
- `POST /api/games/results`
- `GET /api/games/difficulty?game_name=xxx`
- `POST /api/ai/idiom`

## 游戏结果字段说明
`/api/games/results` 统一接收训练结果，字段含义如下：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `game_name` | String | 游戏标识：`penguin_memory` / `schulte_grid` / `schulte_grid_quick` / `mmse_warmup` / `spot_difference` / `forward_reverse_numbers` / `leaf_attention` |
| `score` | Number | 分数 |
| `duration_ms` | Number | 本局耗时（毫秒） |
| `difficulty` | Number | 当前难度（1-N） |
| `accuracy` | Number | 准确率（0~1 或 0~100） |
| `success` | Boolean | 是否通关/完成 |
| `answers` | JSON | 答题/点击明细（见下方示例） |
| `meta` | JSON | 额外信息（可扩展） |

**answers 示例：**
- `penguin_memory`
```json
{
  "target": {"big": 3, "small": 2},
  "input": {"big": 3, "small": 1},
  "correct": false
}
```
- `schulte_grid`
```json
{
  "mode": 2,
  "grid_size": 4,
  "grid_values": [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16],
  "tap_log": [
    {"index": 5, "value": 6, "target": 1, "correct": false, "timestamp": "2026-02-15T03:12:00Z"}
  ],
  "target_idiom": "山清水秀"
}
```
- `schulte_grid_quick`
```json
{
  "grid_size": 4,
  "grid_values": [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16],
  "tap_order": [1,2,3,4]
}
```
- `mmse_warmup`
```json
{
  "items": [
    {"index": 0, "type": "voice", "title": "问题 1/11", "answered_at": "2026-02-15T03:10:00Z"}
  ]
}
```

**meta 常见字段：**
- `level` / `grid` / `mode` / `title`
- `time_limit_sec` / `seconds_left`
- `idiom`
- `accuracy` / `correct`

服务会根据 `success + accuracy` 计算 `next_difficulty`，并在响应里返回。

## 短信实现说明
默认使用内存版本（`utils/sms.go`）。如需阿里云短信实现，使用构建标签启用：

```bash
go build -tags=aliyun_sms .
```

## 开发约定
- 所有受保护接口需携带 `Authorization: Bearer <token>`
- 日期格式使用 `YYYY-MM-DD`
- 提醒时间格式使用 `HH:MM`，多时间用逗号分隔
