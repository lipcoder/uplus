

# UPLUS

用于自动辅助**U+平台**学习的工具

为尊重原接口开发者意愿，已删除的关键接口等，但仍可邮件我以获取

## Features

- 全自动学习
- 支持多账户，且支持多班级（多班级为相较于原项目的升级点）
- 支持多种学习方式（与原版功能一致）
- 账号自动认证

## Dependencies

无额外依赖，原版需要靠nodejs实现的公钥加密账号密码的部分已用crypto/rsa，crypto/rand实现

推荐使用Linux及macOS部署，不推荐Windows，因项目使用了sqlite数据库文件，默认会使用POSIX风格的0600权限模型，此外数据库文件内未进行任何加密处理，请注意环境安全问题

最近测试Windows平台下运行尚可，但仍不推荐，有条件的还是在Linux或macOS部署

## Installation

1. **下载最新 Release**  
   直接前往 [Releases 页面](https://github.com/lipcoder/uplus/releases) 下载最新版本。下载完成后，将可执行文件传输到你的服务器

2. **编译安装**  
   如需自行编译安装，请继续按照下面的步骤操作。

### install & use

1. 打包下载或clone至本地

2. 配置接口

   ```bash
   cp .env.example .env
   chmod 600 .env
   ```

   在 `.env` 中填写五个接口地址。真实的 `.env` 已被 Git 忽略，请另存一份私人备份；发布包和仓库不会携带接口值。
   配置格式为 `KEY=VALUE`，允许空行、整行 `#` 注释、`export ` 前缀以及单引号或双引号包裹的值。URL 查询参数中的 `=` 会完整保留，不执行变量替换或 Shell 命令。
   已设置的系统环境变量优先于 `.env`，也可完全通过系统环境变量配置。程序从**当前工作目录**读取 `.env`，启动前检查五项配置是否完整。
   `UPLUS_COURSE_INFO_URL` 以 `/` 结尾；`UPLUS_SIGN_IN_URL` 以 `?signInId=` 或 `&signInId=` 结尾。

3. 编译并从 `.env` 所在目录启动

   ```bash
   go build -o run cmd/run/main.go
   ./run
   ```

## License

本项目采用 [GNU Affero General Public License v3.0（AGPL-3.0-only）](LICENSE) 授权。