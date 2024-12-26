package speedtest

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Result struct {
	IP          string // IP地址
	Port        int    // 端口
	DataCenter  string // 数据中心
	Region      string // 地区
	City        string // 城市
	Latency     string // 延迟
	TCPDuration int64  // TCP请求延迟
	RespTime    int64
}

const timeout = 1 * time.Second // 超时时间

// CheckColoAndPing 测试地域与连接时间
func CheckColoAndPing(ip string, port int, enableTLS bool, revertcolo string, maxPing int, locationMap map[string]Location) Result {
	client := &http.Client{
		Timeout: timeout + 1,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				dialer := &net.Dialer{
					Timeout:   timeout,
					DualStack: true,
				}
				return dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip, strconv.Itoa(port)))
			},
			DisableKeepAlives: true,
			IdleConnTimeout:   1 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	var connect time.Time
	var connecttime, resptime int64
	trace := &httptrace.ClientTrace{
		ConnectStart: func(network, addr string) { connect = time.Now() },
		ConnectDone: func(network, addr string, err error) {
			connecttime = int64(time.Since(connect) / time.Millisecond)
		},
	}

	protocol := "http://"
	if enableTLS {
		protocol = "https://"
	}
	requestURL := protocol + "speed.cloudflare.com/cdn-cgi/trace"

	req, _ := http.NewRequest("GET", requestURL, nil)
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Close = true

	resp, err := client.Do(req)
	if err != nil {
		return Result{}
	}

	if resp.StatusCode != 200 {
		return Result{}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}
	}
	/*超过最大httpping值，直接跳过*/
	if maxPing > 0 && connecttime > int64(maxPing) {
		return Result{}
	}
	if strings.Contains(string(body), "uag=Mozilla/5.0") {
		if matches := regexp.MustCompile(`colo=([A-Z]+)`).FindStringSubmatch(string(body)); len(matches) > 1 {
			dataCenter := matches[1]
			if revertcolo != "" && dataCenter == revertcolo {
				return Result{}
			}
			loc, ok := locationMap[dataCenter]
			if ok {
				fmt.Printf("发现有效IP %s 位置信息 %s 延迟 %d 毫秒 响应 %d 毫秒\n", ip, loc.City, connecttime, resptime)
				return Result{ip, port, dataCenter, loc.Region, loc.City, fmt.Sprintf("%d ms", connecttime), connecttime, resptime}
			} else {
				fmt.Printf("发现有效IP %s 位置信息未知 延迟 %d 毫秒 响应 %d 毫秒\n", ip, connecttime, resptime)
				return Result{ip, port, dataCenter, "", "", fmt.Sprintf("%d ms", connecttime), connecttime, resptime}
			}
		}
	}
	return Result{}
}

// GetDownloadSpeed 测试下载速度
func GetDownloadSpeed(ip string, port int, enableTLS bool, speedTestURL string) float64 {
	protocol := "http://"
	if enableTLS {
		protocol = "https://"
	}
	fullSpeedTestURL := protocol + speedTestURL

	req, _ := http.NewRequest("GET", fullSpeedTestURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")

	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 0,
	}
	conn, err := dialer.Dial("tcp", net.JoinHostPort(ip, strconv.Itoa(port)))
	if err != nil {
		return 0
	}
	defer conn.Close()

	fmt.Printf("正在测试IP %s 端口 %s\n", ip, strconv.Itoa(port))
	startTime := time.Now()

	client := http.Client{
		Transport: &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				return conn, nil
			},
		},
		Timeout: 5 * time.Second,
	}

	req.Close = true
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("IP %s 端口 %s 测速无效\n", ip, strconv.Itoa(port))
		return 0
	}
	defer resp.Body.Close()

	written, _ := io.Copy(io.Discard, resp.Body)
	duration := time.Since(startTime)
	speed := float64(written) / duration.Seconds() / 1024

	fmt.Printf("IP %s 端口 %s 下载速度 %.0f kB/s\n", ip, strconv.Itoa(port), speed)
	return speed
}
