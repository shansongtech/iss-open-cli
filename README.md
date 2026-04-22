# 项目说明

## 项目概述

闪送开放平台命令行工具,用于协助AI通过命令行方式调用API接口。

## 使用前准备

### 配置文件

在使用前,需要配置 `configs/config.yaml` 文件:

```yaml
api:
  base_url: "https://open.ishansong.com"
  timeout: 3
auth:
  client_id: "your-client-id"              # 替换为您的client_id
  app_secret: "your-app-secret"            # 替换为您的app_secret
  shop_id: "your-shop-id"                  # 替换为您的shop_id
log:
  level: "info"
```

## 许可证

本项目采用 Apache License 2.0 许可证。详见 LICENSE 文件。
