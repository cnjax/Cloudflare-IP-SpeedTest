package speedtest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

type Location struct {
	Iata   string  `json:"iata"`
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	Cca2   string  `json:"cca2"`
	Region string  `json:"region"`
	City   string  `json:"city"`
}

// ReadIPs 从文件中读取IP地址
func ReadIPs(File string, quickmode int) ([]string, error) {
	file, err := os.Open(File)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var ips []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		ipAddr := scanner.Text()
		// 判断是否为 CIDR 格式的 IP 地址
		if strings.Contains(ipAddr, "/") {
			ip, ipNet, err := net.ParseCIDR(ipAddr)
			if err != nil {
				fmt.Printf("无法解析CIDR格式的IP: %v\n", err)
				continue
			}
			if quickmode > 0 {
				maskSize, _ := ipNet.Mask.Size()
				numHosts := 1 << uint(32-maskSize)

				if numHosts <= quickmode {
					// 范围小于等于quickmode,遍历所有IP
					for ip := ip.Mask(ipNet.Mask); ipNet.Contains(ip); inc(ip) {
						ips = append(ips, ip.String())
					}
				} else {
					// 范围大于quickmode,随机选择IP
					r := rand.New(rand.NewSource(time.Now().UnixNano()))
					randomIPs := make(map[string]struct{})

					for len(randomIPs) < quickmode+1 {
						randomOffset := r.Intn(numHosts)
						ip := make(net.IP, len(ipNet.IP))
						copy(ip, ipNet.IP)

						for i := 0; i < len(ip); i++ {
							ip[i] += byte(randomOffset >> (8 * (len(ip) - 1 - i)))
						}

						if ipNet.Contains(ip) {
							randomIPs[ip.String()] = struct{}{}
						}
					}

					for ip := range randomIPs {
						ips = append(ips, ip)
					}
				}
			} else {
				for ip := ip.Mask(ipNet.Mask); ipNet.Contains(ip); inc(ip) {
					ips = append(ips, ip.String())
				}
			}
		} else {
			ips = append(ips, ipAddr)
		}
	}
	return ips, scanner.Err()
}

// inc函数实现ip地址自增
func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// GetLocationMap 获取位置信息映射
func GetLocationMap() map[string]Location {
	var locations []Location
	if _, err := os.Stat("locations.json"); os.IsNotExist(err) {
		fmt.Println("本地 locations.json 不存在\n正在从 https://speed.cloudflare.com/locations 下载 locations.json")
		resp, err := http.Get("https://speed.cloudflare.com/locations")
		if err != nil {
			fmt.Printf("无法从URL中获取JSON: %v\n", err)
			return nil
		}

		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Printf("无法读取响应体: %v\n", err)
			return nil
		}

		err = json.Unmarshal(body, &locations)
		if err != nil {
			fmt.Printf("无法解析JSON: %v\n", err)
			return nil
		}
		file, err := os.Create("locations.json")
		if err != nil {
			fmt.Printf("无法创建文件: %v\n", err)
			return nil
		}
		defer file.Close()

		_, err = file.Write(body)
		if err != nil {
			fmt.Printf("无法写入文件: %v\n", err)
			return nil
		}
	} else {
		fmt.Println("本地 locations.json 已存在,无需重新下载")
		file, err := os.Open("locations.json")
		if err != nil {
			fmt.Printf("无法打开文件: %v\n", err)
			return nil
		}
		defer file.Close()

		body, err := io.ReadAll(file)
		if err != nil {
			fmt.Printf("无法读取文件: %v\n", err)
			return nil
		}

		err = json.Unmarshal(body, &locations)
		if err != nil {
			fmt.Printf("无法解析JSON: %v\n", err)
			return nil
		}
	}

	locationMap := make(map[string]Location)
	for _, loc := range locations {
		locationMap[loc.Iata] = loc
	}
	return locationMap
}
