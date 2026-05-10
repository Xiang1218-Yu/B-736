# 星河资源共享站

## 🛠 技术栈
- Backend: Go + Gin + GORM (单体架构)
- Frontend: Go HTML Templates + CSS + JavaScript
- Database: MySQL 8.0
- Session: Cookie-based Authentication

## 🚀 启动指南 (How to Run)
1. 确保 Docker Desktop 已启动。
2. 在根目录执行：`docker compose up --build`
3. 等待容器启动完成...

## 🔗 服务地址 (Services)
- Web UI: http://localhost:3000
- Database: localhost:3306 (user: root / pass: root)

## 🧪 测试账号
- Admin: admin / 123456

## 📁 项目结构
```
prompt736/
├── main.go                 # 应用入口
├── go.mod / go.sum         # Go 依赖
├── internal/               # 内部模块
│   ├── config/             # 配置管理
│   ├── database/           # 数据库连接
│   ├── models/             # 数据模型
│   ├── middleware/         # 中间件
│   ├── seed/               # 数据初始化
│   └── utils/              # 工具函数
├── templates/              # HTML 模板
│   ├── layouts/            # 基础布局
│   ├── pages/              # 页面模板
│   └── admin/              # 管理后台模板
├── static/                 # 静态资源
│   ├── css/                # 样式文件
│   └── js/                 # JavaScript
├── Dockerfile              # Docker 构建
└── docker-compose.yml      # Docker Compose
```

## ✨ 功能特性
- 用户注册/登录 (Session 认证)
- 每日签到获取积分
- 资源浏览、搜索、筛选
- 资源评论与评分
- 文章浏览与评论
- 管理后台 (用户、资源、文章、分类、标签、配置管理)
- 数据备份与恢复

---

## 🐳 Docker 配置说明

### Dockerfile
```dockerfile
# 构建阶段
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://goproxy.cn,direct
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

# 运行阶段
FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/server .
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/static ./static
EXPOSE 8080
CMD ["./server"]
```
