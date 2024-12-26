package main

import (
	"fmt"
	"io"
	"net"
	"time"
)

// RemoteHosts 是存储远程主机地址的切片
var RemoteHosts = []string{"162.159.140.220:80", "162.159.138.232:80"}

// MaxPing 是最大允许的延迟时间（毫秒）
const MaxPing = 500

// CheckInterval 是检查延迟的时间间隔（秒）
const CheckInterval = 30

// portRedirect 函数实现端口重定向逻辑
func listen() {
	listener, err := net.Listen("tcp", "localhost:8081")
	if err != nil {
		fmt.Println("Failed to start server:", err)
		return
	}
	defer listener.Close()
	fmt.Println("Server started on localhost:8081")

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}

		go PortRedirect(conn)
	}
}

// portRedirect 函数实现端口重定向逻辑
func PortRedirect(localConn net.Conn) {
	defer localConn.Close()

	currentRemoteConn, currentRemoteAddr := connectToNextRemote(localConn)
	if currentRemoteConn == nil {
		fmt.Println("No available remote hosts.")
		return
	}
	defer currentRemoteConn.Close()

	go pingMonitor(currentRemoteConn, currentRemoteAddr, localConn)

	go func() {
		_, err := io.Copy(currentRemoteConn, localConn)
		if err != nil {
			fmt.Println("Error copying data from local to remote:", err)
		}
	}()

	_, err := io.Copy(localConn, currentRemoteConn)
	if err != nil {
		fmt.Println("Error copying data from remote to local:", err)
	}
}

// pingMonitor 监控当前连接的 TCP 延迟
func pingMonitor(currentRemoteConn net.Conn, remoteAddr *net.TCPAddr, localConn net.Conn) {
	ticker := time.NewTicker(CheckInterval * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pingTime := ping(remoteAddr)
			if pingTime > MaxPing {
				fmt.Printf("Ping time %d ms is too high for %s. Switching...\n", pingTime, remoteAddr.String())

				// Attempt to connect to a new remote host
				newRemoteConn, newRemoteAddr := connectToNextRemote(localConn)
				if newRemoteConn != nil {
					fmt.Printf("Switched to new remote host: %s\n", newRemoteAddr.String())

					// Close the old remote connection
					currentRemoteConn.Close()

					// Start redirecting IO for the new connection
					go func() {
						io.Copy(newRemoteConn, localConn)
					}()
					go func() {
						io.Copy(localConn, newRemoteConn)
					}()

					currentRemoteConn = newRemoteConn // Update current connection
					remoteAddr = newRemoteAddr        // Update remote address
				} else {
					fmt.Println("No available remote hosts to switch to.")
				}
			}
		}
	}
}

// connectToNextRemote 连接到下一个可用的远程主机
func connectToNextRemote(localConn net.Conn) (net.Conn, *net.TCPAddr) {
	for _, remoteAddrStr := range RemoteHosts {
		remoteAddr, err := net.ResolveTCPAddr("tcp", remoteAddrStr)
		if err != nil {
			fmt.Println("Failed to resolve remote address:", err)
			continue
		}

		//pingTime := ping(remoteAddr)
		//if pingTime > MaxPing {
		//	fmt.Printf("Ping time %d ms is too high for %s. Trying next...\n", pingTime, remoteAddrStr)
		//	continue
		//}
		//
		remoteConn, err := net.DialTCP("tcp", nil, remoteAddr)
		if err != nil {
			fmt.Println("Failed to connect to remote:", err)
			continue
		}

		fmt.Printf("Connected to %s\n", remoteAddrStr)
		//// 从 RemoteHosts 列表中移除当前远程主机
		//RemoteHosts = append(RemoteHosts[:0], RemoteHosts[1:]...)
		return remoteConn, remoteAddr
	}

	return nil, nil
}

// ping 测试 TCP 延迟
func ping(addr *net.TCPAddr) int {
	startTime := time.Now()
	conn, err := net.DialTimeout("tcp", addr.String(), 1*time.Second)
	if err != nil {
		fmt.Println("Failed to establish connection for ping:", err)
		return -1
	}
	defer conn.Close()

	//_, err = bufio.NewReader(conn).ReadBytes('\n')
	//if err != nil {
	//	fmt.Println("Error reading response:", err)
	//	return -1
	//}
	return int(time.Since(startTime).Milliseconds())
}
