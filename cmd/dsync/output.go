package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

// ANSI 颜色
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorGray   = "\033[90m"
)

// colorEnabled 输出到终端时才着色，管道/重定向时输出纯文本。
var colorEnabled = func() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) == os.ModeCharDevice
}()

func paint(color, s string) string {
	if !colorEnabled {
		return s
	}
	return color + s + colorReset
}

// colorStatus 为同步状态着色：成功绿、失败红、进行中蓝、等待黄。
func colorStatus(status string) string {
	switch strings.ToLower(status) {
	case "success", "completed":
		return paint(colorGreen, status)
	case "failed":
		return paint(colorRed, status)
	case "partial_success":
		return paint(colorYellow, status)
	case "syncing", "running", "retrying":
		return paint(colorBlue, status)
	default:
		return paint(colorGray, status)
	}
}

// printTable 用 tabwriter 输出对齐表格。
func printTable(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
}

// emitJSON 以缩进 JSON 输出（--json 模式）。
func emitJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// shortID 取任务 ID 前 8 位用于表格展示。
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// humanDuration 将时长格式化为 "3m12s" 风格。
func humanDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	return d.Truncate(time.Second).String()
}
