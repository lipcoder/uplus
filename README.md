# UPLUS

用于自动辅助**U+平台**学习的工具

借鉴原作者的设计思路，但最终产品对应博客文章的认证思路，未按照原项目设计

## Features

- 全自动学习
- 支持多账户，且支持多班级（多班级为相较于原项目的升级点）
- 支持多种学习方式（与原版功能一致）
- 支持邮箱推送验证码（相较于原版的升级点，以应对多班级时web界面无法高效显示的问题）
- 账号自动认证

## Dependencies

无额外依赖，原版需要靠nodejs实现的公钥加密账号密码的部分已用crypto/rsa，crypto/rand实现

推荐使用Linux及macOS部署，不推荐Windows，因项目使用了sqlite数据库文件，默认会使用POSIX风格的0600权限模型，此外数据库文件内未进行任何加密处理，请注意环境安全问题

## Installation

1. **下载最新 Release**  
   直接前往 [Releases 页面](https://github.com/lipcoder/uplus/releases) 下载最新版本。下载完成后，将可执行文件传输到你的服务器

2. **编译安装**  
   如需自行编译安装，请继续按照下面的步骤操作。

### install

1. 打包下载或clone至本地

2. ```bash
   go build -o add cmd/run/main.go #日常运行的运行时
   go build -o run cmd/add/main.go #添加账号token，相较于原版，无需手动添加，后续会合并
   ```

## Use

运行时无额外参数，使用邮箱通知是需要配置环境变量QQ_MAIL及QQ_MAIL_AUTH_CODE，可以不使用QQ邮箱，当前我以QQ邮箱为例，需在网页版设置->账号与安全->安全设置->开启smtp服务，获得的授权码为QQ_MAIL_AUTH_CODE的值

```bash
export QQ_MAIL=*****
export QQ_MAIL_AUTH_CODE=********
```

## License

本项目采用和原项目一致的 **PolyForm Noncommercial License 1.0.0** 授权