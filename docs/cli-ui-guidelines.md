# Galaxy CLI UI 规范

所有面向用户的新命令应优先使用 `pkg/cliui`，避免在命令包中自行拼接 ANSI 控制码、表格边框或状态符号。`pkg/cliui` 的布局底层统一使用 Lip Gloss v2；业务命令不得直接依赖第三方表格 API。

## 输出结构

一个完整的交互式命令按需使用以下层次：

1. `Title`：命令目标和是否会修改资源；
2. `Summary`：项目、节点、目标资源等上下文；
3. `Table` / `Checks`：主要数据或检查结果；
4. `Decision`：成功、失败或是否可继续；
5. `Hint`：下一条可执行命令、配置指纹等操作提示。

列表命令应同时提供机器可读格式，例如 `--format json`。表格只用于默认的人类可读格式。

## 状态语义

| 状态 | 符号 | 颜色 | 用途 |
|---|---:|---|---|
| 通过/成功 | `●` / `✓` | 绿色 | 已验证成功 |
| 失败 | `●` / `✗` | 红色 | 阻止当前操作 |
| 警告 | `▲` | 黄色 | 不阻止操作，但需要确认风险 |
| 信息 | `●` | 青色 | 普通进度或说明 |

不要只依赖颜色表达状态；符号和文字必须同时存在，以兼容色觉障碍、日志文件和不支持 ANSI 的终端。

## 终端兼容

- 颜色只在输出目标为 TTY 时启用；
- `NO_COLOR` 非空或 `TERM=dumb` 时禁用颜色；
- 非 TTY 输出保留 Unicode 表格，但不包含 ANSI 控制码；
- JSON 输出不得包含标题、颜色、提示或其他人类可读文本；
- 长路径、镜像名和错误详情必须设置最大列宽并自动换行。
- 列宽以 terminal cell/grapheme 宽度计算，不得使用 `len(string)` 或 rune 数量对齐中文、Emoji 和组合字符。

## 示例

```go
ui := cliui.New(streams.Out)
ui.Title("Compose → Swarm 迁移检查", "只读检查")
ui.Summary([]cliui.Pair{
    {Label: "项目", Value: project},
    {Label: "目标 Stack", Value: stack},
})
ui.Checks(checks)
ui.Decision(canMigrate, "可以迁移")
ui.Hint("下一步", command)
```

交互确认仍使用 `survey`；持续进度可以使用项目现有的 `mpb`。普通列表和检查报告不应引入全屏 TUI，以确保 SSH、CI 和日志采集场景可用。
