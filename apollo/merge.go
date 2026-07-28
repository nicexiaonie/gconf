package apollo

import "fmt"

// mergeSnapshots 将多个 namespace 快照合并为单文件。
// 本期为占位：Merge 模式默认关闭且未实现，启用 Merge 时 Start 会返回错误。
func mergeSnapshots(snaps []Snapshot) (map[string]any, error) {
	return nil, fmt.Errorf("apollo: merge mode not implemented")
}
