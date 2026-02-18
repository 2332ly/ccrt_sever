# API 文档


> **Base URL**: `http://<server-ip>:<port>`

## 1. 通用规范

### 请求头 (Headers)
* 所有 POST 请求必须包含: `Content-Type: application/json`
* 所有受保护的接口 (如 `/api/me`) 必须包含: `Authorization: Bearer <your_token>`

### 响应格式
所有接口返回统一的 JSON 结构。

**成功响应 (HTTP 200):**
直接返回数据对象。
```json
{
  "key": "value"
}
```

**错误响应 (HTTP 4xx / 5xx):**
```json
{
  "error": {
    "code": "ERROR_CODE_STRING",
    "message": "Human readable error message"
  }
}
```
Flutter 端应检查 HTTP 状态码，若非 200，则解析 `error` 字段。

---

## 2. 短信服务

### 2.1 发送短信验证码
在注册或短信登录前调用。

- **URL**: `/api/auth/sms/send`
- **Method**: `POST`

**Request Body (JSON):**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `phone` | String | Yes | 手机号 (6-32位) |

**Success Response (200 OK):**
```json
{
  "verify_id": "sms_12345",
  "verify_code": "1234" // 仅在开发环境/配置开启时返回，正式环境不返回
}
```

**Error Response Example:**
```json
{
  "error": {
    "code": "SMS_SEND_FAILED",
    "message": "Frequency limit reached"
  }
}
```

### 2.2 校验短信验证码 (独立校验)
用于单独校验验证码是否正确（注册和登录接口内部也会自动校验，此接口可选用于分步流程）。

- **URL**: `/api/auth/sms/verify`
- **Method**: `POST`

**Request Body (JSON):**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `phone` | String | Yes | 手机号 |
| `sms_code` | String | Yes | 验证码 (4-8位) |

**Success Response (200 OK):**
```json
{
  "verified": true
}
```

---

## 3. 认证服务

### 3.1 用户注册
注册新用户，需要验证码。

- **URL**: `/api/auth/register`
- **Method**: `POST`

**Request Body (JSON):**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `username` | String | Yes | 用户名 (3-64位) |
| `phone` | String | Yes | 手机号 (必须与发送验证码的手机号一致) |
| `password` | String | Yes | 密码 (6-128位) |
| `sms_code` | String | Yes | 刚收到的短信验证码 |

**Success Response (200 OK):**
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5...",
  "refresh_token": "r1fEoK2xX8z8....",
  "token_type": "Bearer",
  "expires_in": 259200,
  "refresh_expires_in": 2592000,
  "token": "Bearer eyJhbGciOiJIUzI1NiIsInR5..." // 兼容字段
}
```

**Error Response Example:**
- `USER_EXISTS`: 用户名或手机号已存在
- `SMS_VERIFY_FAILED`: 验证码错误或过期

### 3.2 用户登录
支持 **密码登录** 和 **短信验证码登录** 两种模式。

- **URL**: `/api/auth/login`
- **Method**: `POST`

**Request Body (JSON) - 模式一：密码登录**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `username` | String | Optional | 用户名 (username 与 phone 二选一) |
| `phone` | String | Optional | 手机号 |
| `password` | String | Yes | 密码 |

**Request Body (JSON) - 模式二：短信验证码登录**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `phone` | String | Yes | 手机号 |
| `sms_code` | String | Yes | 短信验证码 |

> **注意**: 如果同时传了 `password` 和 `sms_code`，或者都未传，接口会报错 `INVALID_REQUEST`。

**Success Response (200 OK):**
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5...",
  "refresh_token": "r1fEoK2xX8z8....",
  "token_type": "Bearer",
  "expires_in": 259200,
  "refresh_expires_in": 2592000,
  "token": "Bearer eyJhbGciOiJIUzI1NiIsInR5..." // 兼容字段
}
```

---

### 3.3 刷新 Token
使用 refresh_token 换取新的 access_token（并轮换 refresh_token）。

- **URL**: `/api/auth/refresh`
- **Method**: `POST`

**Request Body (JSON):**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `refresh_token` | String | Yes | 刷新令牌 |

**Success Response (200 OK):**
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5...",
  "refresh_token": "r1fEoK2xX8z8....",
  "token_type": "Bearer",
  "expires_in": 259200,
  "refresh_expires_in": 2592000,
  "token": "Bearer eyJhbGciOiJIUzI1NiIsInR5..." // 兼容字段
}
```

## 4. 用户信息

### 4.1 获取当前用户信息
需要携带 Token。

- **URL**: `/api/me`
- **Method**: `GET`
- **Header**: `Authorization: Bearer <token>`

**Success Response (200 OK):**
```json
{
  "username": "user123",
  "emergency_contact_name": "张三",
  "emergency_contact_phone": "13800000000",
  "status": 200
}
```

---

### 4.2 更新紧急联系人
更新当前用户的紧急联系人信息。

- **URL**: `/api/me/emergency`
- **Method**: `PUT`
- **Header**: `Authorization: Bearer <token>`

**Request Body (JSON):**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `name` | String | No | 联系人姓名 |
| `phone` | String | No | 联系电话（建议必填，用于 SOS 拨打） |

**Success Response (200 OK):**
```json
{
  "emergency_contact_name": "张三",
  "emergency_contact_phone": "13800000000"
}
```

---

## 5. 吃药提醒模块

用于管理用户的吃药定时提醒。

前端可以根据 `reminder_time` 和 `frequency` 字段自行实现本地推送通知逻辑。

### 5.1 获取提醒列表
获取当前登录用户的所有吃药提醒。

- **URL**: `/api/medications`
- **Method**: `GET`
- **Header**: `Authorization: Bearer <token>`

**Success Response (200 OK):**
```json
{
  "data": [
    {
      "ID": 1,
      "CreatedAt": "2023-10-27T10:00:00Z",
      "UpdatedAt": "2023-10-27T10:00:00Z",
      "user_id": 10,
      "name": "阿莫西林",
      "dosage": "1粒",
      "frequency": "每日三次",
      "reminder_time": "08:00,12:00,18:00",
      "reminder_channels": "app,sms",
      "alert_style": "strong",
      "start_date": "2023-10-27T00:00:00Z",
      "end_date": "2023-11-03T00:00:00Z",
      "notes": "饭后服用"
    }
  ]
}
```

### 5.2 创建提醒
添加一个新的吃药提醒。

- **URL**: `/api/medications`
- **Method**: `POST`
- **Header**: `Authorization: Bearer <token>`

**Request Body (JSON):**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `name` | String | Yes | 药品名称 |
| `dosage` | String | No | 剂量 (如: 1粒) |
| `frequency` | String | No | 频率描述 (如: 每日一次) |
| `reminder_time` | String | No | 提醒时间 (逗号分隔, 如 "08:00,20:00") |
| `reminder_channels` | String | No | 提醒渠道 (逗号分隔, 如 "app,sms,voice") |
| `alert_style` | String | No | 提醒强度 (strong/normal) |
| `start_date` | String | Yes | 开始日期 (Format: YYYY-MM-DD) |
| `end_date` | String | No | 结束日期 (Format: YYYY-MM-DD) |
| `notes` | String | No | 备注 |

**Example Request:**
```json
{
  "name": "维生素C",
  "dosage": "1片",
  "frequency": "每天一次",
  "reminder_time": "09:00",
  "reminder_channels": "app,sms",
  "alert_style": "strong",
  "start_date": "2023-10-27",
  "notes": "早餐后"
}
```

**Success Response (200 OK):**
```json
{
  "id": 2
}
```

### 5.3 更新提醒
修改已存在的提醒。

- **URL**: `/api/medications/:id`
- **Method**: `PUT`
- **Header**: `Authorization: Bearer <token>`

**Request Body (JSON):**
同创建接口。注意：`end_date` 如果传空字符串或不传，若逻辑支持可视为清除结束日期(具体看后端实现)，当前实现若不传则不更新该字段? 不，PUT通常是全量覆盖或部分更新。本接口实现逻辑为：如果传了字段则更新。

**Example Request:**
```json
{
  "name": "维生素C (增强版)",
  "dosage": "2片",
  "frequency": "每天一次",
  "reminder_time": "09:00",
  "reminder_channels": "app,sms,voice",
  "alert_style": "normal",
  "start_date": "2023-10-27",
  "end_date": "2023-12-31",
  "notes": "改为每次两片"
}
```

**Success Response (200 OK):**
```json
{
  "success": true
}
```

---


### 5.5 获取指定日期的用药计划
返回当天所有提醒时间点及打卡状态。

- **URL**: `/api/medications/schedule?date=YYYY-MM-DD`
- **Method**: `GET`
- **Header**: `Authorization: Bearer <token>`

**Success Response (200 OK):**
```json
{
  "date": "2026-02-15",
  "items": [
    {
      "medication_id": 1,
      "name": "阿莫西林",
      "dosage": "1粒",
      "frequency": "每日三次",
      "reminder_time": "08:00,12:00,18:00",
      "reminder_channels": "app,sms",
      "alert_style": "strong",
      "scheduled_date": "2026-02-15",
      "scheduled_time": "08:00",
      "scheduled_at": "2026-02-15T08:00:00+08:00",
      "status": "pending"
    }
  ]
}
```

### 5.6 服药打卡
对某个计划时间点进行“已服/跳过”标记。

- **URL**: `/api/medications/checkins`
- **Method**: `POST`
- **Header**: `Authorization: Bearer <token>`

**Request Body (JSON):**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `medication_id` | Number | Yes | 提醒ID |
| `scheduled_date` | String | Yes | 计划日期 (YYYY-MM-DD) |
| `scheduled_time` | String | Yes | 计划时间 (HH:MM) |
| `status` | String | Yes | `taken` / `skipped` |
| `notes` | String | No | 备注 |

**Success Response (200 OK):**
```json
{
  "checkin": {
    "ID": 12,
    "medication_id": 1,
    "scheduled_date": "2026-02-15T00:00:00Z",
    "scheduled_time": "08:00",
    "status": "taken",
    "taken_at": "2026-02-15T08:05:00Z"
  }
}
```

### 5.7 服药统计
返回最近一段时间的打卡统计。

- **URL**: `/api/medications/stats?range=week`
- **Method**: `GET`
- **Header**: `Authorization: Bearer <token>`
- **Query**: `range` (week/month)，或 `days=7`

**Success Response (200 OK):**
```json
{
  "range": "week",
  "start_date": "2026-02-09",
  "end_date": "2026-02-15",
  "summary": {
    "total": 21,
    "taken": 18,
    "skipped": 1,
    "pending": 2,
    "adherence": 0.857,
    "streak_current": 3,
    "streak_best": 5
  },
  "daily": [
    { "date": "2026-02-15", "total": 3, "taken": 3, "skipped": 0, "pending": 0 }
  ]
}
```

---

## 6. MMSE 量表

### 6.1 获取当前激活量表与题目

- **URL**: `/api/mmse/scale`
- **Method**: `GET`
- **Header**: `Authorization: Bearer <token>`

**Success Response (200 OK):**
```json
{
  "scale": { "id": 1, "name": "MMSE", "total_score": 30 },
  "version": { "id": 1, "name": "MMSE", "version": "2001", "total_score": 30, "is_active": true },
  "modules": [
    {
      "id": 1,
      "name": "定向力",
      "max_score": 10,
      "sort_order": 1,
      "questions": [
        { "id": 1, "type": "fields_correct", "content": "现在是：年/月/日/星期/季节（每项1分）", "max_score": 5 }
      ]
    }
  ]
}
```

### 6.2 提交 MMSE 评估并自动评分

- **URL**: `/api/mmse/assessments`
- **Method**: `POST`
- **Header**: `Authorization: Bearer <token>`

**Request Body (JSON):**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `scale_version_id` | Number | No | 指定版本，不传则默认激活版本 |
| `answers` | Array | Yes | 题目作答列表 |

**Answer Item:**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `question_id` | Number | Yes | 题目ID |
| `user_answer` | JSON | No | 结构化答案 |
| `manual_score` | Number | No | 手动计分题目必填 |

**Example Request:**
```json
{
  "answers": [
    { "question_id": 1, "user_answer": { "correct_fields": ["year","month","day"] } },
    { "question_id": 3, "user_answer": { "items": ["花园","国旗"] } },
    { "question_id": 4, "user_answer": { "sequence": [93,86,79,72,65] } },
    { "question_id": 8, "manual_score": 1 }
  ]
}
```

**Success Response (200 OK):**
```json
{
  "assessment_id": 123,
  "total_score": 26,
  "level": "mild"
}
```

### 6.3 获取评估详情

- **URL**: `/api/mmse/assessments/:id`
- **Method**: `GET`
- **Header**: `Authorization: Bearer <token>`

**Success Response (200 OK):**
```json
{
  "assessment": {
    "id": 123,
    "scale_version_id": 1,
    "total_score": 26,
    "level": "mild",
    "completed_at": "2026-02-14T10:00:00Z"
  },
  "answers": [
    { "id": 1, "assessment_id": 123, "question_id": 1, "score": 3, "user_answer": "{\"correct_fields\":[\"year\",\"month\",\"day\"]}" }
  ],
  "module_scores": [
    { "id": 1, "assessment_id": 123, "module_id": 1, "score": 8 }
  ]
}
```

### 6.4 获取评估列表（分页）

- **URL**: `/api/mmse/assessments`
- **Method**: `GET`
- **Header**: `Authorization: Bearer <token>`
- **Query**: `page` (default 1), `size` (default 20, max 100)

**Success Response (200 OK):**
```json
{
  "data": [
    { "id": 123, "scale_version_id": 1, "total_score": 26, "level": "mild", "completed_at": "2026-02-14T10:00:00Z" }
  ],
  "page": 1,
  "size": 20,
  "total": 1
}
```

### 6.5 AI 评分与评价

- **URL**: `/api/mmse/assessments/:id/ai`
- **Method**: `POST`
- **Header**: `Authorization: Bearer <token>`

**Request Body (JSON):**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `force` | Boolean | No | 强制重新调用 AI（默认 false） |

**Success Response (200 OK):**
```json
{
  "assessment_id": 123,
  "ai_score": 25,
  "ai_level": "mild",
  "ai_comment": "AI comment ...",
  "ai_result": "{\"ai_score\":25,\"ai_level\":\"mild\",\"comment\":\"...\",\"suggestion\":\"...\"}",
  "ai_model": "gpt-4.1-mini",
  "ai_at": "2026-02-14T10:10:00Z"
}
```

---

## 7. 康复训练游戏

### 7.1 提交游戏结果

- **URL**: `/api/games/results`
- **Method**: `POST`
- **Header**: `Authorization: Bearer <token>`

**Request Body (JSON):**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `game_name` | String | Yes | 游戏标识 |
| `score` | Number | No | 分数 |
| `duration_ms` | Number | No | 耗时（毫秒） |
| `difficulty` | Number | No | 当前难度 |
| `accuracy` | Number | No | 准确率（0~1 或 0~100） |
| `success` | Boolean | No | 是否成功 |
| `answers` | JSON | No | 答题/点击明细（结构自定义） |
| `meta` | JSON | No | 额外信息（结构自定义） |

**Example Request:**
```json
{
  "game_name": "schulte_grid",
  "score": 120,
  "duration_ms": 38000,
  "difficulty": 3,
  "accuracy": 0.82,
  "success": true,
  "answers": {
    "grid_size": 4,
    "tap_log": [
      {"index": 5, "value": 6, "target": 1, "correct": false}
    ]
  },
  "meta": {
    "mode": 2,
    "title": "高阶·闪现",
    "seconds_left": 12
  }
}
```

**Success Response (200 OK):**
```json
{
  "id": 88,
  "game_name": "schulte_grid",
  "score": 120,
  "duration_ms": 38000,
  "difficulty": 3,
  "accuracy": 0.82,
  "success": true,
  "next_difficulty": 4,
  "min_difficulty": 1,
  "max_difficulty": 6
}
```

### 7.2 获取推荐难度

- **URL**: `/api/games/difficulty?game_name=xxx`
- **Method**: `GET`
- **Header**: `Authorization: Bearer <token>`

**Success Response (200 OK):**
```json
{
  "game_name": "schulte_grid",
  "difficulty": 3,
  "last_difficulty": 2,
  "last_accuracy": 0.88,
  "last_success": true,
  "min_difficulty": 1,
  "max_difficulty": 6
}
```

---

## 8. AI 服务

### 8.1 生成 AI 成语（舒尔特方格）

- **URL**: `/api/ai/idiom`
- **Method**: `POST`
- **Header**: `Authorization: Bearer <token>`

**Request Body (JSON):**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `temperature` | Number | No | 采样温度，默认 0.9 |

**Success Response (200 OK):**
```json
{
  "idiom": "山清水秀",
  "chars": ["山", "清", "水", "秀"]
}
```

---

## 9. 紧急呼救

### 9.1 记录 SOS 事件
前端在发起紧急呼救时上报事件，并返回本次拨号的信息。

- **URL**: `/api/sos`
- **Method**: `POST`
- **Header**: `Authorization: Bearer <token>`

**Request Body (JSON):**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `note` | String | No | 备注（最多 200 字，超出将截断） |

**Success Response (200 OK):**
```json
{
  "id": 1,
  "contact_name": "张三",
  "contact_phone": "13800000000",
  "created_at": "2026-02-16T10:00:00Z"
}
```

**Error Response Example:**
- `EMERGENCY_CONTACT_MISSING`: 未设置紧急联系人手机号

### 5.4 删除提醒
删除指定的提醒。

- **URL**: `/api/medications/:id`
- **Method**: `DELETE`
- **Header**: `Authorization: Bearer <token>`

**Success Response (200 OK):**
```json
{
  "success": true
}
```
