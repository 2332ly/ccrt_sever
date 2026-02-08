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
  "token": "eyJhbGciOiJIUzI1NiIsInR5..." // JWT Token，后续请求需携带
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
  "token": "eyJhbGciOiJIUzI1NiIsInR5..."
}
```

---

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
  "status": 200
}
```

