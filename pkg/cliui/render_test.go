package cliui

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"
)

func TestChecksRenderAsUnicodeTableWithoutANSIForPlainOutput(t *testing.T) {
	var output bytes.Buffer
	ui := NewForTest(&output, false, 100)

	ui.Checks([]Check{
		{Status: StatusPass, Name: "Compose 配置", Detail: "有效"},
		{Status: StatusFail, Name: "Stack 配置", Detail: "端口无效"},
	})

	require.Contains(t, output.String(), "╭")
	require.Contains(t, output.String(), "● 通过")
	require.Contains(t, output.String(), "● 失败")
	require.NotContains(t, output.String(), "\x1b[")
}

func TestChecksUseStatusColorsWhenEnabled(t *testing.T) {
	var output bytes.Buffer
	ui := NewForTest(&output, true, 100)

	ui.Checks([]Check{{Status: StatusWarn, Name: "存储", Detail: "本地卷"}})

	require.Contains(t, output.String(), "\x1b[33m▲ 警告\x1b[0m")
}

func TestSummaryAndDecision(t *testing.T) {
	var output bytes.Buffer
	ui := NewForTest(&output, false, 100)

	ui.Title("迁移检查", "只读操作")
	ui.Summary([]Pair{{Label: "项目", Value: "mysql"}, {Label: "目录", Value: "/srv/mysql"}})
	ui.Decision(true, "可以迁移")

	require.Contains(t, output.String(), "◆ 迁移检查")
	require.Contains(t, output.String(), "mysql")
	require.Contains(t, output.String(), "✓ 可以迁移")
}

func TestSummaryKeepsMixedCJKEnglishLabelOnOneLine(t *testing.T) {
	var output bytes.Buffer
	ui := NewForTest(&output, false, 120)

	ui.Summary([]Pair{
		{Label: "源容器", Value: "redis"},
		{Label: "目标 Stack", Value: "redis"},
		{Label: "目标 Service", Value: "redis"},
	})

	require.Contains(t, output.String(), "目标 Service │ redis")
	require.NotContains(t, output.String(), "\n│ Service")
	require.Equal(t, 2, strings.Count(output.String(), "\n├"), "three summary rows must have two separators")

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	expectedWidth := lipgloss.Width(lines[0])
	for _, line := range lines {
		require.Equal(t, expectedWidth, lipgloss.Width(line), "summary line is not aligned: %q", line)
	}
}

func TestCJKTableBordersHaveConsistentTerminalCellWidth(t *testing.T) {
	var output bytes.Buffer
	ui := NewForTest(&output, false, 96)

	ui.Table([]Column{
		{Header: "状态", WidthMax: 10, Align: AlignCenter},
		{Header: "检查项", WidthMax: 24},
		{Header: "详情", WidthMax: 60},
	}, [][]string{
		{"● 通过", "端口配置规范化", "已将 MySQL 的 published 端口转换为整数"},
		{"▲ 警告", "本地存储", "命名卷只存在于当前机器，请确认调度约束"},
		{"● 失败", "兼容性", "包含中文、English 和 Emoji 🚀 的混合文本"},
	})

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	require.Greater(t, len(lines), 5)
	expectedWidth := lipgloss.Width(lines[0])
	for _, line := range lines {
		require.Equal(t, expectedWidth, lipgloss.Width(line), "table line is not aligned: %q", line)
	}
}
