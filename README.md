# ccrt_sever

基于 Go + Gin 的认知康复后端服务，负责账号认证、用药管理、MMSE 评估、训练游戏结果上报，以及 AI 驱动的认知画像与训练闭环。

当前版本已经把比赛演示最关键的三条链路打通：

1. MMSE 评估 -> 认知画像
2. 认知画像 -> 个性化训练计划
3. 训练结果 -> 统一康复分 -> 难度调整 -> 训练效果分析

## 技术栈

- Go 1.24
- Gin
- GORM + MySQL
- JWT
- Viper
- OpenAI / DeepSeek 兼容大模型接口
- DashScope TTS
- Aliyun NLS ASR

## 主要能力

- 用户注册、登录、短信验证码、Token 刷新
- 用户资料与用药管理
- MMSE 量表拉取、提交、自动评分、AI 解读
- 训练游戏结果上报
- 统一训练难度分配
- 统一康复表现打分 `rehab_score`
- AI 认知画像
- 个性化训练计划
- 训练趋势、训练统计、阶段性 AI 报告
- AI 成语、生活助手、TTS、ASR

## 难度与打分闭环

训练难度不再只是“上一局对了就加一档”，而是三层决策：

- `baseline_difficulty`
  - 由认知画像分数和近 5 次同游戏历史融合得到
- `target_band_low / target_band_high`
  - 默认围绕 baseline 形成训练带，避免难度抖动
- `next_difficulty`
  - 结合最近表现 EWMA、本局 `rehab_score`、连续高分/低分情况做在线调档

统一康复分 `rehab_score` 为 0-100，服务端统一计算：

- `accuracy` 权重 0.55
- `completion` 权重 0.20
- `time_efficiency` 权重 0.15
- `stability` 权重 0.10

这套分数会同时用于：

- `GET /api/games/difficulty`
- `POST /api/games/results`
- `GET /api/cognitive-profile`
- `GET /api/training-plan/current`
- `GET /api/analytics/cognitive-trend`
- `GET /api/analytics/training-summary`

## Errorless Learning 约束

服务端已支持训练游戏统一的温柔辅助元数据：

- `assist_triggered`
- `assist_level_max`
- `assist_resolution`
- `softened_next_round`

调档时会额外执行保护约束：

- 触发 Level 2 辅助或 `assist_resolution != independent` 时，本轮最高只能持平，不能升档
- `completed_with_guidance` 或 `unfinished` 允许下一局降 1 档
- `GET /api/analytics/training-summary` 和 `game_breakdown` 会返回 `assist_sessions`、`assist_rate`、`avg_assist_level`

## 目录结构

- `main.go`
  - 服务入口
- `router/router.go`
  - 路由注册
- `config/`
  - 配置加载、数据库初始化、MMSE 种子数据
- `controllers/`
  - `user_controller.go`：认证与用户信息
  - `medication_controller.go`：用药管理
  - `mmse_controller.go`：MMSE 评估与评分
  - `game_controller.go`：训练结果、难度分配、康复分
  - `cognitive_controller.go`：认知画像、训练计划、训练分析、AI 报告
  - `ai_controller.go`：成语、生活助手、TTS、ASR
- `models/`
  - 数据模型
- `services/`
  - 调度器等服务逻辑
- `utils/`
  - JWT、OpenAI、短信、通用响应工具

## 配置

配置文件：`config/config.yml`

常用配置项：

- `app.port`
- `database.dsn`
- `jwt.secret`
- `sms.*`
- `openai.*`
- `dashscope.*`
- `aliyunNls.*`

支持用环境变量覆盖：

- `JWT_SECRET`
- `DASHSCOPE_BASE_URL`
- `DASHSCOPE_API_KEY`
- `DASHSCOPE_MODEL`
- `DASHSCOPE_VOICE`
- `ALIYUN_NLS_ACCESS_KEY_ID`
- `ALIYUN_NLS_ACCESS_KEY_SECRET`
- `ALIYUN_NLS_APP_KEY`
- `ALIYUN_NLS_TOKEN_URL`
- `ALIYUN_NLS_ASR_URL`

## 启动

1. 准备 MySQL 并配置 `database.dsn`
2. 配置 `jwt.secret`
3. 如需 AI 能力，补齐 OpenAI / DashScope / Aliyun NLS 配置
4. 启动服务

```bash
cd ccrt_sever
go run .
```

如果未配置端口，服务会回退到 `:8080`。

## API 概览

### 认证与用户

- `POST /api/auth/register`
- `POST /api/auth/login`
- `POST /api/auth/refresh`
- `POST /api/auth/sms/send`
- `POST /api/auth/sms/verify`
- `GET /api/me`

### 用药

- `GET /api/medications`
- `POST /api/medications`
- `GET /api/medications/schedule`
- `POST /api/medications/checkins`
- `GET /api/medications/stats`
- `GET /api/medications/:id`
- `PUT /api/medications/:id`
- `DELETE /api/medications/:id`

### MMSE

- `GET /api/mmse/scale`
- `POST /api/mmse/assessments`
- `GET /api/mmse/assessments`
- `GET /api/mmse/assessments/:id`
- `POST /api/mmse/assessments/:id/ai`

### 游戏训练

- `GET /api/games/difficulty?game_name=xxx`
- `POST /api/games/results`

`GET /api/games/difficulty` 当前会返回：

- `difficulty`
- `baseline_difficulty`
- `target_band_low`
- `target_band_high`
- `difficulty_source`
- `reason`
- `last_difficulty`
- `last_accuracy`
- `last_success`
- `last_rehab_score`

`POST /api/games/results` 当前会返回：

- `score`
- `accuracy`
- `rehab_score`
- `score_breakdown`
- `next_difficulty`
- `next_difficulty_reason`
- `baseline_difficulty`
- `target_band_low`
- `target_band_high`
- `difficulty_source`

### AI 认知闭环

- `GET /api/cognitive-profile`
- `POST /api/training-plan/generate`
- `GET /api/training-plan/current`
- `GET /api/analytics/cognitive-trend?days=N`
- `GET /api/analytics/training-summary?days=N`
- `POST /api/analytics/ai-report`

训练计划中的 `recommended_games` 当前包含：

- `game`
- `display_name`
- `reason`
- `start_difficulty`
- `target_difficulty_low`
- `target_difficulty_high`
- `daily_sessions`
- `suggested_difficulty`

其中 `suggested_difficulty` 为兼容字段，值等于 `start_difficulty`。

### AI 能力

- `POST /api/ai/idiom`
- `POST /api/ai/life-assistant`
- `POST /api/ai/tts`
- `POST /api/ai/asr`

## 验证

```bash
cd ccrt_sever
go test ./...
```

当前已经补上 `controllers/` 下的关键回归测试，覆盖：

- 训练趋势兼容字段
- 训练统计兼容字段
- 难度调档逻辑
- 康复分计算回退逻辑

## 备注

- 需要携带 `Authorization: Bearer <token>`
- 日期格式：`YYYY-MM-DD`
- 时间格式：`HH:MM`
- 若本地开启短信实现，可使用：

```bash
go build -tags=aliyun_sms .
```
