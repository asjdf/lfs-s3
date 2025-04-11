# lfs-s3

一个基于 S3 存储的 Git LFS 服务器实现。

## 功能特性

- 支持 S3 兼容的存储后端
- 可配置的认证机制
- 支持日志收集（支持 CLS）
- 支持 Sentry 错误监控
- 可配置的缓存机制

## 快速开始

### 配置

复制 `config.example.yaml` 为 `config.yaml` 并修改配置：

```yaml
mode: debug
port: "8080"
log:
    logPath: ""
    cls:
        endpoint: ""
        accessKey: ""
        accessToken: ""
        topicID: ""
sentryDsn: ""
lfsS3:
    s3:
        externalEndpoint: ""
        endpoint: ""
        accessKeyID: ""
        secretAccessKey: ""
        bucketName: ""
        region: ""
        pathStyle: false
    auth:
        enableCache: false
```

### 运行

#### 普通用户运行

项目已经实现了自动化版本发布，发布的镜像可以在 [GitHub Packages](https://github.com/asjdf/lfs-s3/pkgs/container/lfs-s3) 查看。

```bash
# 拉取最新版本
docker pull ghcr.io/asjdf/lfs-s3:latest

# 运行容器
docker run -d -p 8080:8080 -v /path/to/config.yaml:/app/config.yaml ghcr.io/asjdf/lfs-s3:latest
```

#### 开发者调试运行

```bash
# 克隆项目
git clone https://github.com/asjdf/lfs-s3.git
cd lfs-s3

# 运行项目
go run main.go
```

### Docker 构建（开发者）

```bash
# 构建镜像
docker build -t lfs-s3 .

# 运行容器
docker run -d -p 8080:8080 lfs-s3
```

## 项目结构

```
.
├── cmd/                # 命令行入口
│   ├── config/        # 自动生成配置
│   ├── server/        # 服务器实现
│   └── init.go        # 初始化命令
├── mod/               # 模块实现
├── docs/              # 文档
├── config.yaml        # 配置文件
└── main.go            # 主程序入口
```

## 许可证

本项目采用 Apache License 2.0 许可证，详见 [LICENSE](LICENSE) 文件。