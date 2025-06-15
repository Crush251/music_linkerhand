package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type CanMessage struct {
	Interface string `json:"interface"`
	Id        uint32 `json:"id"`
	Data      []byte `json:"data"`
}
type PianoConfig struct {
	Interfaces struct {
		LeftHand  string `json:"leftHand"`
		RightHand string `json:"rightHand"`
		LeftArm   string `json:"leftArm"`
		RightArm  string `json:"rightArm"`
	} `json:"interfaces"`
	MusicData MusicData `json:"musicData"`
}

type MusicData struct {
	DefaultPosition struct {
		Left  ArmPosition `json:"left"`
		Right ArmPosition `json:"right"`
	} `json:"default_position"`
	Music []MusicNote `json:"music"`
}

type ArmPosition struct {
	X    int `json:"x"`
	Y    int `json:"y"`
	Z    int `json:"z"`
	Move int `json:"move"` // 机械臂移动几个单位
}

// ArmMovement 表示机械臂的移动指令
type ArmMovement struct {
	X int `json:"x"` // X轴移动量
	Y int `json:"y"` // Y轴移动量
}

// HandAction 表示单手的动作指令
type HandAction struct {
	Fingers []string    `json:"fingers"` // 要活动的手指列表
	Move    ArmMovement `json:"move"`    // 机械臂移动指令
	Time    []float64   `json:"time"`    // 每个手指的动作时间(秒)
}

// MusicNote 表示一个完整的音乐节拍指令
type MusicNote struct {
	Index int        `json:"index"` // 节拍序号
	Left  HandAction `json:"left"`  // 左手动作
	Right HandAction `json:"right"` // 右手动作
}

// 定义位姿请求结构体
type PoseRequest struct {
	Interface string `json:"interface"`
	X         int    `json:"x"` // 已经是0.001mm单位
	Y         int    `json:"y"`
	Z         int    `json:"z"`
	RX        int    `json:"rx"` // 已经是0.001°单位
	RY        int    `json:"ry"`
	RZ        int    `json:"rz"`
	Speed     int    `json:"speed"` // 0-100
}

// 手指映射
var fingerIndexMap = map[string]int{
	"index":  2,
	"middle": 3,
	"ring":   4,
	"pinky":  5,
}

// 手指弹琴预设值
var O7FingerPianoPreset = []byte{0, 255, 235, 235, 235, 235, 100}
var L10FingerPianoPreset = []byte{0, 0, 225, 225, 225, 225}

// 左臂弹琴预设值，对应xyzrxyz
var leftArmPianoPreset = []int{400, 0, 251, 0, 80, 0}

// 右臂弹琴预设值，对应xyzrxyz
var rightArmPianoPreset = []int{400, 0, 240, 0, 85, 0}

var L10currentleftFinger = []byte{0, 0, 225, 225, 225, 225}
var L10currentrightFinger = []byte{0, 0, 225, 225, 225, 225}
var O7currentleftFinger = []byte{0, 255, 235, 235, 235, 235, 100}
var O7currentrightFinger = []byte{0, 255, 235, 235, 235, 235, 100}

// 机械臂和灵巧手canid
var LeftHand = "can0"
var RightHand = "can1"
var LeftArm = "can2"
var RightArm = "can3"

var killPiano = false
var stopPiano = false

type ArmPianoPresetReq struct {
	Side   string `json:"side"`   // "left" or "right"
	Values []int  `json:"values"` // 6个关节/位姿
}

type FingerPianoPresetReq struct {
	Values []byte `json:"values"`
}

func main() {
	r := gin.Default()
	// 静态文件服务
	r.Static("/static", "./static")
	r.GET("/", func(c *gin.Context) {
		c.File("./static/index.html")
	})

	// ====================== 机械臂路由组 (/api/arm/*) ======================
	armGroup := r.Group("/api/arm")
	{
		// 发送关节控制指令
		armGroup.POST("/send_joint", func(c *gin.Context) {
			var req struct {
				Interface string `json:"interface"`
				J1        int    `json:"j1"` // 已经是0.001°单位
				J2        int    `json:"j2"`
				J3        int    `json:"j3"`
				J4        int    `json:"j4"`
				J5        int    `json:"j5"`
				J6        int    `json:"j6"`
				Speed     int    `json:"speed"` // 0-100
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
				return
			}

			// 验证关节角度范围
			if !validateJointRange(req.J1, -154000, 154000) ||
				!validateJointRange(req.J2, 0, 195000) ||
				!validateJointRange(req.J3, -175000, 0) ||
				!validateJointRange(req.J4, -102000, 102000) ||
				!validateJointRange(req.J5, -75000, 75000) ||
				!validateJointRange(req.J6, -120000, 120000) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "joint angle out of range"})
				return
			}

			// 验证速度范围
			if req.Speed < 0 || req.Speed > 100 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "speed must be 0-100"})
				return
			}

			// 发送J1-J2 (0x155)
			data155 := intPairToBytes(req.J1, req.J2)
			msgj12 := CanMessage{
				Interface: req.Interface,
				Id:        0x155,
				Data:      data155,
			}
			if err := forwardToCanService(msgj12); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "send J1-J2 failed", "details": err.Error()})
				return
			}

			// 发送J3-J4 (0x156)
			data156 := intPairToBytes(req.J3, req.J4)
			msgj34 := CanMessage{
				Interface: req.Interface,
				Id:        0x156,
				Data:      data156,
			}
			if err := forwardToCanService(msgj34); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "send J3-J4 failed", "details": err.Error()})
				return
			}

			// 发送J5-J6 (0x157)
			data157 := intPairToBytes(req.J5, req.J6)
			msgj56 := CanMessage{
				Interface: req.Interface,
				Id:        0x157,
				Data:      data157,
			}
			if err := forwardToCanService(msgj56); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "send J5-J6 failed", "details": err.Error()})
				return
			}

			// 发送运动控制指令 (0x151)
			controlMsg := CanMessage{
				Interface: req.Interface, // 使用第一个接口发送控制指令
				Id:        0x151,
				Data: []byte{
					0x01,            // 控制模式
					0x01,            // 关节控制
					byte(req.Speed), // 速度
					0, 0, 0, 0, 0,   // 后面的字节默认为0
				},
			}
			if err := forwardToCanService(controlMsg); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "send control command failed", "details": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"status": "joint commands sent"})
		})

		// 使能
		armGroup.POST("/enable", func(c *gin.Context) {
			req := CanMessage{}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
				return
			}

			msg := CanMessage{
				Interface: req.Interface,
				Id:        0x471,
				Data:      []byte{0x07, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, // 使能全部关节
			}
			if err := forwardToCanService(msg); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "enable failed", "details": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"status": "enabled"})
		})

		// 失能
		armGroup.POST("/disable", func(c *gin.Context) {
			req := CanMessage{}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
				return
			}
			msg := CanMessage{
				Interface: req.Interface,
				Id:        0x471,
				Data:      []byte{0x07, 0x01, 0, 0, 0, 0, 0, 0}, // 失能全部关节
			}
			if err := forwardToCanService(msg); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "disable failed", "details": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"status": "disabled"})
		})

		// 回零
		armGroup.POST("/to_zero", func(c *gin.Context) {
			req := CanMessage{}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
				return
			}
			msgj12 := CanMessage{
				Interface: req.Interface,
				Id:        0x155,
				Data:      []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			}

			if err := forwardToCanService(msgj12); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "to zero failed", "details": err.Error()})
				return
			}
			msgj34 := CanMessage{
				Interface: req.Interface,
				Id:        0x156,
				Data:      []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			}
			if err := forwardToCanService(msgj34); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "to zero failed", "details": err.Error()})
				return
			}
			msgj56 := CanMessage{
				Interface: req.Interface,
				Id:        0x157,
				Data:      []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			}
			if err := forwardToCanService(msgj56); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "to zero failed", "details": err.Error()})
				return
			}
			// 发送运动控制指令 (0x151)
			controlMsg := CanMessage{
				Interface: req.Interface, // 使用第一个接口发送控制指令
				Id:        0x151,
				Data: []byte{
					0x01,          // 控制模式
					0x01,          // 关节控制
					0x64,          // 速度
					0, 0, 0, 0, 0, // 后面的字节默认为0
				},
			}
			if err := forwardToCanService(controlMsg); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "send control command failed", "details": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"status": "to zero"})
		})

		// 发送位姿指令的路由处理
		armGroup.POST("/send_pose", func(c *gin.Context) {
			var req PoseRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
				return
			}
			fmt.Println("req: ", req)
			if err := sendPoseCommand(req.X, req.Y, req.Z, req.RX, req.RY, req.RZ, req.Speed, req.Interface); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"status": "pose commands sent"})
		})

		// 更新手臂弹琴预设值
		armGroup.POST("/arm_piano_preset", func(c *gin.Context) {
			var req ArmPianoPresetReq
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(400, gin.H{"error": "invalid request"})
				return
			}
			if len(req.Values) != 6 {
				c.JSON(400, gin.H{"error": "must be 6 values"})
				return
			}
			if req.Side == "left" {
				leftArmPianoPreset = req.Values
				fmt.Println("leftArmPianoPreset: ", leftArmPianoPreset)
			} else if req.Side == "right" {
				rightArmPianoPreset = req.Values
				fmt.Println("rightArmPianoPreset: ", rightArmPianoPreset)
			}

			c.JSON(200, gin.H{"status": "success"})
		})
	}
	// ====================== 手指路由组 (/api/hand/*) ======================
	handGroup := r.Group("/api/hand")
	{
		handGroup.POST("/o7/control", func(c *gin.Context) {
			var msg CanMessage
			if err := json.NewDecoder(c.Request.Body).Decode(&msg); err != nil {
				http.Error(c.Writer, fmt.Sprintf("请求解析失败: %v", err), http.StatusBadRequest)
				return
			}
			if len(msg.Data) != 8 {
				http.Error(c.Writer, "数据长度错误", http.StatusBadRequest)
				return
			}
			fmt.Println("msg: ", msg)
			if err := forwardToCanService(msg); err != nil {
				http.Error(c.Writer, fmt.Sprintf("发送失败: %v", err), http.StatusInternalServerError)
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "success"})
		})
		// 更新手指弹琴预设值
		handGroup.POST("/o7/piano_preset", func(c *gin.Context) {
			// 获取请求体中的数据
			var values FingerPianoPresetReq
			if err := c.ShouldBindJSON(&values); err != nil {
				c.JSON(400, gin.H{"error": "invalid request"})
				return
			}
			if len(values.Values) != 7 {
				c.JSON(400, gin.H{"error": "must be 7 values"})
				return
			}
			O7FingerPianoPreset = values.Values
			fmt.Println("O7FingerPianoPreset: ", O7FingerPianoPreset)
			c.JSON(200, gin.H{"status": "success"})
		})
		handGroup.POST("/o7/speed", func(c *gin.Context) {
			var msg CanMessage
			if err := json.NewDecoder(c.Request.Body).Decode(&msg); err != nil {
				http.Error(c.Writer, fmt.Sprintf("请求解析失败: %v", err), http.StatusBadRequest)
				return
			}
			if err := forwardToCanService(msg); err != nil {
				http.Error(c.Writer, fmt.Sprintf("发送失败: %v", err), http.StatusInternalServerError)
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "success"})
		})
		handGroup.POST("/l10/control", func(c *gin.Context) {
			var msg CanMessage
			if err := json.NewDecoder(c.Request.Body).Decode(&msg); err != nil {
				http.Error(c.Writer, fmt.Sprintf("请求解析失败: %v", err), http.StatusBadRequest)
				return
			}
			if err := forwardToCanService(msg); err != nil {
				http.Error(c.Writer, fmt.Sprintf("发送失败: %v", err), http.StatusInternalServerError)
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "success"})
		})
		handGroup.POST("/l10/speed", func(c *gin.Context) {
			var msg CanMessage
			if err := json.NewDecoder(c.Request.Body).Decode(&msg); err != nil {
				http.Error(c.Writer, fmt.Sprintf("请求解析失败: %v", err), http.StatusBadRequest)
				return
			}
			if err := forwardToCanService(msg); err != nil {
				http.Error(c.Writer, fmt.Sprintf("发送失败: %v", err), http.StatusInternalServerError)
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "success"})
		})
		handGroup.POST("/l10/piano_preset", func(c *gin.Context) {
			var values FingerPianoPresetReq
			if err := c.ShouldBindJSON(&values); err != nil {
				c.JSON(400, gin.H{"error": "invalid request"})
				return
			}
			L10FingerPianoPreset = values.Values
			fmt.Println("L10FingerPianoPreset: ", L10FingerPianoPreset)
			c.JSON(200, gin.H{"status": "success"})
		})

	}
	// ====================== 钢琴演奏路由组 (/api/piano/*) ======================
	pianoGroup := r.Group("/api/piano")
	{
		pianoGroup.POST("/start", func(c *gin.Context) {
			var config PianoConfig
			if err := c.ShouldBindJSON(&config); err != nil {
				c.JSON(400, gin.H{"error": "invalid request"})
				return
			}
			//fmt.Println("config: ", config)
			// 启动钢琴演奏，调用函数playPiano
			playPiano(config)
			c.JSON(200, gin.H{"status": "success"})
		})
		pianoGroup.POST("/stop", func(c *gin.Context) {
			// 暂停发送，修改全局变量以暂停发送
			stopPiano = true
			fmt.Println("stop")
			c.JSON(200, gin.H{"status": "success"})
		})
		//恢复发送，修改全局变量以恢复发送
		pianoGroup.POST("/resume", func(c *gin.Context) {
			stopPiano = false
			fmt.Println("resume")
			c.JSON(200, gin.H{"status": "success"})
		})
		pianoGroup.POST("/kill", func(c *gin.Context) {
			// 终止发送，修改全局变量以停止发送
			killPiano = true
			fmt.Println("kill")
			c.JSON(200, gin.H{"status": "success"})
		})

	}

	// 查询CAN设备接口
	r.GET("/api/can_interfaces", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"interfaces": QueryNumberofCanDevices()})
	})
	fmt.Println("server running on port http://localhost:6130")
	r.Run(":6130")

}

// 发送位姿指令的函数
func sendPoseCommand(x, y, z, rx, ry, rz, speed int, canId string) error {
	// 验证速度范围
	if speed < 0 || speed > 100 {
		return fmt.Errorf("speed must be 0-100")
	}

	// 发送X-Y (0x152)
	data152 := intPairToBytes(x, y)
	msgxy := CanMessage{
		Interface: canId,
		Id:        0x152,
		Data:      data152,
	}
	if err := forwardToCanService(msgxy); err != nil {
		return fmt.Errorf("send X-Y failed: %v", err)
	}

	// 发送Z-RX (0x153)
	data153 := intPairToBytes(z, rx)
	msgzrx := CanMessage{
		Interface: canId,
		Id:        0x153,
		Data:      data153,
	}
	if err := forwardToCanService(msgzrx); err != nil {
		return fmt.Errorf("send Z-RX failed: %v", err)
	}

	// 发送RY-RZ (0x154)
	data154 := intPairToBytes(ry, rz)
	msgryrz := CanMessage{
		Interface: canId,
		Id:        0x154,
		Data:      data154,
	}
	if err := forwardToCanService(msgryrz); err != nil {
		return fmt.Errorf("send RY-RZ failed: %v", err)
	}

	// 发送运动控制指令 (0x151)
	controlMsg := CanMessage{
		Interface: canId,
		Id:        0x151,
		Data: []byte{
			0x01,          // 控制模式
			0x00,          // 点位控制
			byte(speed),   // 速度
			0, 0, 0, 0, 0, // 后面的字节默认为0
		},
	}
	if err := forwardToCanService(controlMsg); err != nil {
		return fmt.Errorf("send control command failed: %v", err)
	}

	return nil
}

// index , middle , ring , pinky。分别代表食指，中指，无名指，小指。
// 手指下压幅度，所有手指共用一个
var fingerDown = int(255 * 0.6)

// 钢琴演奏函数，设定好预设值，然后开始演奏
func playPiano(config PianoConfig) {
	//绑定canid
	LeftHand = config.Interfaces.LeftHand
	RightHand = config.Interfaces.RightHand
	LeftArm = config.Interfaces.LeftArm
	RightArm = config.Interfaces.RightArm

	fmt.Println("LeftHand: ", LeftHand, "RightHand: ", RightHand, "LeftArm: ", LeftArm, "RightArm: ", RightArm)
	//将手臂移动到预设位置
	if config.MusicData.DefaultPosition != (struct {
		Left  ArmPosition `json:"left"`
		Right ArmPosition `json:"right"`
	}{}) {
		movedefault(config.MusicData.DefaultPosition)
		fmt.Println("已经移动到预设位置！！马上准备演奏！！")
	} else {
		fmt.Println("没有预设位置，请手动调整预设位置")
	}

	// 发送music的序列
	playmusic(config.MusicData.Music)
}

// 移动到预设位置
func movedefault(default_position struct {
	Left  ArmPosition `json:"left"`
	Right ArmPosition `json:"right"`
}) {
	//兼容自定义好的预设位置
	// fmt.Println("default_position: ", default_position)
	// // 发送左臂的序列,在预设的x,y,z,rx,ry,rz的基础上，加上预设的值
	// leftArmPianoPreset[0] += default_position.Left.X
	// leftArmPianoPreset[1] += default_position.Left.Y
	// leftArmPianoPreset[2] += default_position.Left.Z

	// //发送左臂的序列
	// sendPoseCommand(leftArmPianoPreset[0], leftArmPianoPreset[1], leftArmPianoPreset[2], leftArmPianoPreset[3], leftArmPianoPreset[4], leftArmPianoPreset[5], 100, LeftArm)

	// // 发送右臂的序列,在预设的x,y,z,rx,ry,rz的基础上，加上预设的值
	// rightArmPianoPreset[0] += default_position.Right.X
	// rightArmPianoPreset[1] += default_position.Right.Y
	// rightArmPianoPreset[2] += default_position.Right.Z

	// //发送右臂的序列
	// sendPoseCommand(rightArmPianoPreset[0], rightArmPianoPreset[1], rightArmPianoPreset[2], rightArmPianoPreset[3], rightArmPianoPreset[4], rightArmPianoPreset[5], 100, RightArm)
	//手动调整版本
	leftArmPianoPreset[2] += default_position.Left.Move * 21
	rightArmPianoPreset[2] += default_position.Right.Move * 21
	sendArmPoseCommand(LeftArm, leftArmPianoPreset)
	sendArmPoseCommand(RightArm, rightArmPianoPreset)
}

// index , middle , ring , pinky。分别代表食指，中指，无名指，小指。
var LEFT_HAND_ID uint32 = 0x28
var RIGHT_HAND_ID uint32 = 0x27

func playmusic(music []MusicNote) {
	// 初始化当前手指和机械臂位姿
	copy(L10currentleftFinger, L10FingerPianoPreset)
	copy(L10currentrightFinger, L10FingerPianoPreset)

	for _, note := range music {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			handleOneSide(note.Left, &L10currentleftFinger, &leftArmPianoPreset, LeftHand, LeftArm, RIGHT_HAND_ID)
			wg.Done()
		}()
		go func() {
			handleOneSide(note.Right, &L10currentrightFinger, &rightArmPianoPreset, RightHand, RightArm, RIGHT_HAND_ID)
			wg.Done()
		}()
		wg.Wait()
		fmt.Println("note: ", note)
	}
}

func handleOneSide(action HandAction, fingerState *[]byte, armPose *[]int, handCan string, armCan string, handId uint32) {
	var wg sync.WaitGroup
	wg.Add(len(action.Fingers))
	// 1. 并发执行所有手指动作
	for i, fingerName := range action.Fingers {
		go func(idx int, name string, duration float64) {
			// 按压
			(*fingerState)[fingerIndexMap[name]] = byte(fingerDown)
			sendL10FingerCommand(handCan, *fingerState, handId)
			// 按压持续
			time.Sleep(time.Duration(duration * float64(time.Second)))
			// 恢复
			(*fingerState)[fingerIndexMap[name]] = L10FingerPianoPreset[fingerIndexMap[name]]
			sendL10FingerCommand(handCan, *fingerState, handId)
			wg.Done()
		}(i, fingerName, action.Time[i])
	}
	// 2. 等待所有手指动作完成
	wg.Wait()
	// 3. 机械臂移动
	(*armPose)[0] += action.Move.X * 21
	(*armPose)[1] += action.Move.Y * 21
	sendArmPoseCommand(armCan, *armPose)
	// 4. 固定休眠0.2s，模拟机械臂移动时间
	time.Sleep(150 * time.Millisecond)
}

func sendArmPoseCommand(armCan string, armPose []int) {
	sendPoseCommand(armPose[0]*1000, armPose[1]*1000, armPose[2]*1000, armPose[3]*1000, armPose[4]*1000, armPose[5]*1000, 100, armCan)
}

func sendL10FingerCommand(handCan string, fingerState []byte, handId uint32) {
	msg := CanMessage{
		Interface: handCan,
		Id:        handId, //是left的话，id是0x28，是right的话，id是0x27
		Data:      append([]byte{0x01}, fingerState...),
	}
	forwardToCanService(msg)
}

// 将两个int32转换为8字节数据 (每个int32占4字节，已经是0.001°单位)
func intPairToBytes(val1, val2 int) []byte {
	data := make([]byte, 8)

	// 直接使用输入值，因为已经是0.001°单位
	int1 := int32(val1)
	int2 := int32(val2)

	// 第一个值放在前4字节
	binary.BigEndian.PutUint32(data[0:4], uint32(int1))
	// 第二个值放在后4字节
	binary.BigEndian.PutUint32(data[4:8], uint32(int2))

	return data
}

// 查询CAN设备列表
func QueryNumberofCanDevices() []string {
	type ResponseData struct {
		Count      int      `json:"count"`
		Interfaces []string `json:"interfaces"`
	}

	type ApiResponse struct {
		Status string       `json:"status"`
		Data   ResponseData `json:"data"`
	}

	resp, err := http.Get("http://localhost:5260/api/setup/available")
	if err != nil {
		log.Printf("获取CAN设备列表失败: %v", err)
		return []string{}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("读取响应失败: %v", err)
		return []string{}
	}

	var apiResponse ApiResponse
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		log.Printf("解析JSON失败: %v", err)
		return []string{}
	}

	return apiResponse.Data.Interfaces
}

// 验证关节角度范围
func validateJointRange(value, min, max int) bool {
	return value >= min && value <= max
}

// 转发到本地CAN服务
func forwardToCanService(msg CanMessage) error {
	jsonData, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message failed: %v", err)
	}

	resp, err := http.Post("http://localhost:5260/api/can", "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("send to CAN service failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("CAN service error: %s", string(body))
	}

	return nil
}
