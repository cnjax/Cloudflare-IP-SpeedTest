package main

import (
	"Cloudflare-IP-SpeedTest/speedtest"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"
)

const (
	requestURL = "speed.cloudflare.com/cdn-cgi/trace" // 请求trace URL
	timeout    = 1 * time.Second                      // 超时时间
	//maxDuration = 2 * time.Second                      // 最大持续时间
)

var (
	File         = flag.String("file", "ip.txt", "IP地址文件名称")  // IP地址文件名称
	outFile      = flag.String("outfile", "ip.csv", "输出文件名称") // 输出文件名称
	defaultPort  = flag.Int("port", 80, "端口")                 // 端口
	quickmode    = flag.Int("quick", 0, "快递模式，每个子网挑个数，默认为0")  // 端口
	maxThreads   = flag.Int("max", 100, "并发请求最大协程数")          // 最大协程数
	maxPing      = flag.Int("ping", 100, "最大ping值不超过多少ms")
	round        = flag.Int("round", 1, "httpping测试轮数")                                        // 最大协程数
	speedTest    = flag.Int("speedtest", 0, "下载测速协程数量,设为0禁用测速")                                // 下载测速协程数量
	speedTestURL = flag.String("url", "speed.cloudflare.com/__down?bytes=500000000", "测速文件地址") // 测速文件地址
	enableTLS    = flag.Bool("tls", false, "是否启用TLS")                                          // TLS是否启用
	revertcolo   = flag.String("revertcolo", "", "是否反转colo,填入KIX，排除revertcolo部分，默认为空")         // 是否反转颜色      = flag.String("outfile", "ip.csv", "输出文件名称")                                  // 输出文件名称
)

type speedtestresult struct {
	speedtest.Result
	downloadSpeed float64
}

var locationMap map[string]speedtest.Location

func main() {
	flag.Parse()
	if *defaultPort == 80 {
		*enableTLS = false
	}
	startTime := time.Now()

	locationMap = speedtest.GetLocationMap()
	if locationMap == nil {
		fmt.Println("无法获取地理位置信息，请检查网络连接或重试。")
		return
	}

	// 第一轮使用文件中的IP
	ips, err := speedtest.ReadIPs(*File, *quickmode)
	if err != nil {
		fmt.Printf("无法从文件中读取 IP: %v\n", err)
		return
	}

	// 存储最终结果
	var finalResults []speedtestresult

	// 进行多轮测试
	for currentRound := 1; currentRound <= *round; currentRound++ {
		if currentRound > 1 {
			fmt.Printf("\n开始第 %d 轮测试...\n", currentRound)
			// 使用上一轮的有效IP
			var newIPs []string
			for _, result := range finalResults {
				newIPs = append(newIPs, result.IP)
			}
			ips = newIPs
			finalResults = []speedtestresult{} // 清空上一轮结果
		}

		var wg sync.WaitGroup
		wg.Add(len(ips))

		resultChan := make(chan speedtest.Result, len(ips))
		thread := make(chan struct{}, *maxThreads)

		var count int
		total := len(ips)

		for _, ip := range ips {
			thread <- struct{}{}
			go func(ip string) {
				defer func() {
					<-thread
					wg.Done()
					count++
					percentage := float64(count) / float64(total) * 100
					fmt.Printf("第 %d 轮已完成: %d 总数: %d 进度: %.2f%%\r", currentRound, count, total, percentage)
					if count == total {
						fmt.Printf("第 %d 轮已完成: %d 总数: %d 进度: %.2f%%\n", currentRound, count, total, percentage)
					}
				}()
				res := speedtest.CheckColoAndPing(ip, *defaultPort, *enableTLS, *revertcolo, *maxPing, locationMap)
				if res.Latency != "" {
					resultChan <- res
				}
			}(ip)
		}

		wg.Wait()
		close(resultChan)

		if len(resultChan) == 0 {
			fmt.Printf("\n第 %d 轮测试未发现有效的IP\n", currentRound)
			break
		}

		// 处理当前轮次的结果
		var results []speedtestresult
		for res := range resultChan {
			results = append(results, speedtestresult{Result: res})
		}

		// 按TCP延迟排序
		sort.Slice(results, func(i, j int) bool {
			return results[i].TCPDuration < results[j].TCPDuration
		})

		finalResults = results
	}

	// 所有轮次完成后，如果需要进行速度测试
	if *speedTest > 0 && len(finalResults) > 0 {
		fmt.Printf("\n开始下载速度测试...\n")
		var speedResults []speedtestresult
		var wg2 sync.WaitGroup
		wg2.Add(*speedTest)
		count := 0
		total := len(finalResults)
		resultChan := make(chan speedtestresult, total)

		thread := make(chan struct{}, *maxThreads)
		for i := 0; i < *speedTest; i++ {
			thread <- struct{}{}
			go func() {
				defer func() {
					<-thread
					wg2.Done()
				}()
				for _, res := range finalResults {
					downloadSpeed := speedtest.GetDownloadSpeed(res.IP, res.Port, *enableTLS, *speedTestURL)
					resultChan <- speedtestresult{
						Result:        res.Result,
						downloadSpeed: downloadSpeed,
					}

					count++
					percentage := float64(count) / float64(total) * 100
					fmt.Printf("速度测试完成: %.2f%%\r", percentage)
					if count == total {
						fmt.Printf("速度测试完成: %.2f%%\n", percentage)
					}
				}
			}()
		}
		wg2.Wait()
		close(resultChan)

		// 收集速度测试结果
		for res := range resultChan {
			speedResults = append(speedResults, res)
		}

		// 按下载速度排序
		sort.Slice(speedResults, func(i, j int) bool {
			return speedResults[i].downloadSpeed > speedResults[j].downloadSpeed
		})

		finalResults = speedResults
	}

	// 输出结果到文件
	file, err := os.Create(*outFile)
	if err != nil {
		fmt.Printf("无法创建文件: %v\n", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	if *speedTest > 0 {
		writer.Write([]string{"IP地址", "端口", "TLS", "数据中心", "地区", "城市", "网络延迟", "下载速度"})
	} else {
		writer.Write([]string{"IP地址", "端口", "TLS", "数据中心", "地区", "城市", "网络延迟"})
	}

	for _, res := range finalResults {
		if *speedTest > 0 {
			writer.Write([]string{res.IP, strconv.Itoa(res.Port), strconv.FormatBool(*enableTLS), res.DataCenter, res.Region, res.City, res.Latency, fmt.Sprintf("%.0f kB/s", res.downloadSpeed)})
		} else {
			writer.Write([]string{res.IP, strconv.Itoa(res.Port), strconv.FormatBool(*enableTLS), res.DataCenter, res.Region, res.City, res.Latency})
		}
	}

	writer.Flush()
	fmt.Print("\033[2J")
	fmt.Printf("成功将结果写入文件 %s，共测试 %d 轮，耗时 %d秒\n", *outFile, *round, time.Since(startTime)/time.Second)
}
