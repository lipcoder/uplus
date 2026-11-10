

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

**下载最新 Release**  
	直接前往 [Releases 页面](https://github.com/lipcoder/uplus/releases) 下载最新版本。下载完成后，将可执行文件传输到你的服务器

## License

本项目采用 [GNU Affero General Public License v3.0（AGPL-3.0-only）](LICENSE) 授权。