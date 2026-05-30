# nyanya-trip-route-track Server

nyanya-trip-route-track 后端服务，提供 API 接口支持。

## 🌐 在线体验

- **Web 应用**：https://trip.aiiko.club/
- **App 下载**：https://trip.aiiko.club/download

## ✨ 功能特性

- 📍 **位置服务** - 实时位置追踪与存储
- 🗺️ **行程管理** - 行程路线的 CRUD 操作
- 🏙️ **城市足迹** - 记录访问过的城市
- 📖 **路书服务** - 路书生成与管理
- 🤖 **AI 功能** - 集成 OpenAI 提供智能服务
- 🌤️ **天气查询** - 实时天气信息获取（Open-Meteo）
- 📝 **记忆系统** - 行程记忆与旅途记忆
- 🛡️ **隐私围栏** - 地理位置隐私保护
- 🗄️ **文件管理** - 支持文件上传与存储
- 🚗 **车辆管理** - 车辆信息管理
- 🔍 **POI 搜索** - 兴趣点搜索功能

## 🛠️ 技术栈

- **语言** - Go 1.25
- **Web 框架** - Gin
- **数据库** - MongoDB + Redis
- **向量数据库** - Qdrant
- **AI** - OpenAI API / GLM
- **实时通信** - Socket.IO
- **认证** - Saki SSO
- **协议** - Protocol Buffers

## 🚀 快速开始

```bash
# 安装依赖
go mod download

# 开发模式（使用 air 热重载）
air

# 或直接运行
go run main.go
```

## 📂 项目结构

```
server/
├── main.go             # 入口文件
├── controllers/        # 控制器
├── routers/            # 路由
├── models/             # 数据模型
├── dbx/                # 数据库操作
├── services/           # 业务逻辑
├── config/             # 配置
├── db/                 # 数据库连接
└── protos/             # Protocol Buffers
```

## ⚙️ 环境配置

创建 `config.dev.json` 配置文件：

```json
{
  "server": {
    "port": 23203
  },
  "mongodb": {
    "uri": "mongodb://user:password@host:port/db"
  },
  "redis": {
    "addr": "host:port",
    "password": "your-password"
  },
  "qdrant": {
    "grpcUrl": "host:443",
    "apiKey": "your-api-key"
  }
}
```

## 🌐 API 文档

### 主要接口

| 模块 | 路径 | 说明 |
|------|------|------|
| 行程 | `/api/v1/trip` | 行程相关 CRUD |
| AI | `/api/v1/ai` | AI 功能接口 |
| 城市 | `/api/v1/city` | 城市足迹 |
| 路书 | `/api/v1/roadbook` | 路书管理 |
| 记忆 | `/api/v1/journey-memory` | 旅途记忆 |
| 导航 | `/api/v1/navigation` | 导航服务 |
| 位置 | `/api/v1/position` | 位置数据 |
| 车辆 | `/api/v1/vehicle` | 车辆信息 |

## 🔗 相关链接

- [在线体验](https://trip.aiiko.club/)
- [App 下载](https://trip.aiiko.club/download)
- [Web 前端](../trip-route-track-web)
- [Android App](../trip-route-track-flutter-app)

## 📄 License

MIT
