# 🎹 钢琴演奏控制系统

## 项目简介
本项目是一个基于 Web 的多 CAN 设备钢琴演奏控制平台，支持灵巧手和机械臂的原子操作、位姿/关节控制、混合序列解析等，适用于机器人弹琴、远程控制等场景。

## 主要功能
- 多 CAN 设备自动识别与管理
- 灵巧手原子操作序列控制、手指滑块控制、弹琴预设
- 机械臂关节/位姿控制、原子序列批量运动、弹琴预设
- 支持左/右手、左/右臂区分
- 混合序列解析与一键发送
- 现代美观的前端交互界面

## 前端操作说明
1. **CAN接口配置**：自动检测可用CAN接口，选择后可配置设备类型。
2. **手部控制**：
   - 选择左/右手，支持原子操作序列输入与滑块控制。
   - 可一键更新手指弹琴预设值。
3. **机械臂控制**：
   - 选择左/右臂，支持关节模式(J)和位姿模式(P)切换。
   - 位姿模式下可一键更新弹琴预设值。
   - 支持回零、设置零点、急停、恢复等操作。
4. **混合控制**：支持手和机械臂混合序列输入与发送。

## 后端接口说明（部分）
- `/api/can_interfaces`：获取可用CAN接口列表
- `/api/hand/control`：手指滑块控制
- `/api/hand/atomic`：手部原子操作序列
- `/api/hand/fingers_piano_preset`：更新手指弹琴预设
- `/api/arm/send_joint`：机械臂关节控制
- `/api/arm/send_pose`：机械臂位姿控制
- `/api/arm/y_sequence`：机械臂Y序列批量运动
- `/api/arm/arm_piano_preset`：更新机械臂弹琴预设
- `/api/arm/enable|disable|emergency_stop|emergency_resume|to_zero|set_zero`：机械臂基础操作

## 如何运行
1. 安装依赖：
   - Go 1.18+
   - Node.js（如需前端构建）
2. 启动后端：
   ```bash
   go run main.go
   ```
3. 浏览器访问 [http://localhost:6120](http://localhost:6120)

## 依赖环境
- Go 1.18 及以上
- Gin Web 框架
- 浏览器支持 ES6/Fetch API
- 后端需能访问本地 CAN 服务（默认 5260 端口）

## 联系方式
- 作者：lxqs
- 邮箱：lxqs@example.com
- Issues/PR 欢迎提交！ 

## 项目简介
本项目是一个基于 Web 的多 CAN 设备钢琴演奏控制平台，支持灵巧手和机械臂的原子操作、位姿/关节控制、混合序列解析等，适用于机器人弹琴、远程控制等场景。

## 主要功能
- 多 CAN 设备自动识别与管理
- 灵巧手原子操作序列控制、手指滑块控制、弹琴预设
- 机械臂关节/位姿控制、原子序列批量运动、弹琴预设
- 支持左/右手、左/右臂区分
- 混合序列解析与一键发送
- 现代美观的前端交互界面

## 前端操作说明
1. **CAN接口配置**：自动检测可用CAN接口，选择后可配置设备类型。
2. **手部控制**：
   - 选择左/右手，支持原子操作序列输入与滑块控制。
   - 可一键更新手指弹琴预设值。
3. **机械臂控制**：
   - 选择左/右臂，支持关节模式(J)和位姿模式(P)切换。
   - 位姿模式下可一键更新弹琴预设值。
   - 支持回零、设置零点、急停、恢复等操作。
4. **混合控制**：支持手和机械臂混合序列输入与发送。

## 后端接口说明（部分）
- `/api/can_interfaces`：获取可用CAN接口列表
- `/api/hand/control`：手指滑块控制
- `/api/hand/atomic`：手部原子操作序列
- `/api/hand/fingers_piano_preset`：更新手指弹琴预设
- `/api/arm/send_joint`：机械臂关节控制
- `/api/arm/send_pose`：机械臂位姿控制
- `/api/arm/y_sequence`：机械臂Y序列批量运动
- `/api/arm/arm_piano_preset`：更新机械臂弹琴预设
- `/api/arm/enable|disable|emergency_stop|emergency_resume|to_zero|set_zero`：机械臂基础操作

## 如何运行
1. 安装依赖：
   - Go 1.18+
   - Node.js（如需前端构建）
2. 启动后端：
   ```bash
   go run main.go
   ```
3. 浏览器访问 [http://localhost:6120](http://localhost:6120)

## 依赖环境
- Go 1.18 及以上
- Gin Web 框架
- 浏览器支持 ES6/Fetch API
- 后端需能访问本地 CAN 服务（默认 5260 端口）

## 联系方式
- 作者：lxqs
- 邮箱：lxqs@example.com
- Issues/PR 欢迎提交！ 
 