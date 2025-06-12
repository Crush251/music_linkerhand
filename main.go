package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type CanMessage struct {
	Interface string `json:"interface"`
	Id        uint32 `json:"id"`
	Data      []byte `json:"data"`
}

// 使能状态
var enabled = false

func main() {
	r := gin.Default()

	// 静态文件服务
	r.Static("/static", "./static")
	r.GET("/", func(c *gin.Context) {
		c.File("./static/index.html")
	})
	interfaces := []string{"can2", "can3"}
	// ====================== 机械臂路由组 (/api/arm/*) ======================
	armGroup := r.Group("/api/arm")
	{
		// 发送关节控制指令
		armGroup.POST("/send_joint", func(c *gin.Context) {
			if !enabled {
				c.JSON(http.StatusBadRequest, gin.H{"error": "not enabled"})
				return
			}
			var req struct {
				J1    int `json:"j1"` // 已经是0.001°单位
				J2    int `json:"j2"`
				J3    int `json:"j3"`
				J4    int `json:"j4"`
				J5    int `json:"j5"`
				J6    int `json:"j6"`
				Speed int `json:"speed"` // 0-100
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
			for _, iface := range interfaces {
				// 发送J1-J2 (0x155)
				data155 := intPairToBytes(req.J1, req.J2)

				msgj12 := CanMessage{
					Interface: iface,
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
					Interface: iface,
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
					Interface: iface,
					Id:        0x157,
					Data:      data157,
				}
				if err := forwardToCanService(msgj56); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "send J5-J6 failed", "details": err.Error()})
					return
				}

				// 发送运动控制指令 (0x151)
				controlMsg := CanMessage{
					Interface: iface, // 使用第一个接口发送控制指令
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
			}
			c.JSON(http.StatusOK, gin.H{"status": "joint commands sent"})
		})

		// 使能
		armGroup.POST("/enable", func(c *gin.Context) {
			enabled = true
			for _, iface := range interfaces {
				msg := CanMessage{
					Interface: iface,
					Id:        0x471,
					Data:      []byte{0x07, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, // 使能全部关节
				}
				if err := forwardToCanService(msg); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "enable failed", "details": err.Error()})
					return
				}
			}
			c.JSON(http.StatusOK, gin.H{"status": "enabled"})
		})

		// 失能
		armGroup.POST("/disable", func(c *gin.Context) {
			enabled = false
			for _, iface := range interfaces {
				msg := CanMessage{
					Interface: iface,
					Id:        0x471,
					Data:      []byte{0x07, 0x01, 0, 0, 0, 0, 0, 0}, // 失能全部关节
				}
				if err := forwardToCanService(msg); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "disable failed", "details": err.Error()})
					return
				}
			}
			c.JSON(http.StatusOK, gin.H{"status": "disabled"})
		})

		// 回零
		armGroup.POST("/to_zero", func(c *gin.Context) {
			for _, iface := range interfaces {
				msgj12 := CanMessage{
					Interface: iface,
					Id:        0x155,
					Data:      []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
				}

				if err := forwardToCanService(msgj12); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "to zero failed", "details": err.Error()})
					return
				}
				msgj34 := CanMessage{
					Interface: iface,
					Id:        0x156,
					Data:      []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
				}
				if err := forwardToCanService(msgj34); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "to zero failed", "details": err.Error()})
					return
				}
				msgj56 := CanMessage{
					Interface: iface,
					Id:        0x157,
					Data:      []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
				}
				if err := forwardToCanService(msgj56); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "to zero failed", "details": err.Error()})
					return
				}
				// 发送运动控制指令 (0x151)
				controlMsg := CanMessage{
					Interface: iface, // 使用第一个接口发送控制指令
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
			}
			c.JSON(http.StatusOK, gin.H{"status": "to zero"})
		})

		// Y序列批量运动接口

		armGroup.POST("/y_sequence", func(c *gin.Context) {
			if !enabled {
				c.JSON(http.StatusBadRequest, gin.H{"error": "not enabled"})
				return
			}
			var req struct {
				Sequence  [][]int `json:"sequence"`  // [[y, t], ...]
				Interface string  `json:"interface"` // can2/can3
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
				return
			}
			// 固定参数
			x := 400000
			z := 170000
			rx := 0
			ry := 80000
			rz := 0
			canIface := req.Interface
			if canIface == "" {
				canIface = "can2" // 默认can2
			}
			fmt.Println("req.Sequence: ", req.Sequence, "interface:", canIface)
			for _, pair := range req.Sequence {
				if len(pair) != 2 {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sequence element"})
					return
				}
				y := pair[0]
				t := pair[1]
				if y < -70000 || y > 70000 {
					c.JSON(http.StatusBadRequest, gin.H{"error": "y out of range"})
					return
				}
				// 1. 发送X-Y
				data152 := intPairToBytes(x, y)
				msgxy := CanMessage{
					Interface: canIface,
					Id:        0x152,
					Data:      data152,
				}
				if err := forwardToCanService(msgxy); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "send X-Y failed", "details": err.Error()})
					return
				}

				// 2. 发送Z-RX
				data153 := intPairToBytes(z, rx)
				msgzrx := CanMessage{
					Interface: canIface,
					Id:        0x153,
					Data:      data153,
				}
				if err := forwardToCanService(msgzrx); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "send Z-RX failed", "details": err.Error()})
					return
				}

				// 3. 发送RY-RZ
				data154 := intPairToBytes(ry, rz)
				msgryrz := CanMessage{
					Interface: canIface,
					Id:        0x154,
					Data:      data154,
				}
				if err := forwardToCanService(msgryrz); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "send RY-RZ failed", "details": err.Error()})
					return
				}

				// 发送运动控制指令 (0x151)
				controlMsg := CanMessage{
					Interface: canIface, // 使用指定接口发送控制指令
					Id:        0x151,
					Data: []byte{
						0x01,          // 控制模式
						0x00,          // 点位控制
						100,           // 速度
						0, 0, 0, 0, 0, // 后面的字节默认为0
					},
				}
				if err := forwardToCanService(controlMsg); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "send control command failed", "details": err.Error()})
					return
				}

				// 4. 停留
				if t > 0 {
					time.Sleep(time.Duration(t) * time.Millisecond)
				}
			}
			c.JSON(http.StatusOK, gin.H{"status": "sequence sent"})
		})
	}
	// ====================== 手指路由组 (/api/hand/*) ======================
	handGroup := r.Group("/api/hand")
	{
		handGroup.POST("/control", func(c *gin.Context) {
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
		handGroup.POST("/atomic", func(c *gin.Context) {
			canMessage := CanMessage{
				Interface: "can0",
				Id:        0x27,
				Data:      []byte{0x01, 128, 128, 128, 128, 128, 128, 128},
			}
			if err := forwardToCanService(canMessage); err != nil {
				http.Error(c.Writer, fmt.Sprintf("发送失败: %v", err), http.StatusInternalServerError)
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "success"})
		})
	}

	// 查询CAN设备接口
	r.GET("/api/can_interfaces", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"interfaces": QueryNumberofCanDevices()})
	})
	fmt.Println("server running on port http://localhost:6120")
	r.Run(":6120")
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
