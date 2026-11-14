// ============================================================================
// ⑤ 这条行为的测试：语音提示「未用过不出现、用过每轮都在、不串会话」
// ----------------------------------------------------------------------------
// 原文：voice_used_test.go（宿主程序根目录，整文件逐字搬入，仅在本头部加了说明）
//
// 这个测试值得一起看的原因：它把上面 ④ 的三条口头承诺变成了可执行的断言——
//   · 未置位 → 快照里不能出现"语音输入"（不无中生有）
//   · 置位   → 必须出现"使用过语音输入"与"同音错字"（提示内容不能漂）
//   · 同状态两次组装 → 逐字相同（粘性提示不闪变，差分引擎才不会反复发更新）
//   · 另一个会话未置位 → 不受影响（不串会话）
//
// 注意：它是 package main 的测试，依赖宿主的 App/envPrompt 等类型，
// 在参考项目里不能单独 go test（缺宿主其余文件），只作阅读材料。
// ============================================================================

package main

import (
	"strings"
	"testing"
)

// 语音听写粘性标记（SessionMeta.VoiceUsed）：环境快照只在置位后注入同音错字
// 提示行，未置位绝不出现；置位由 chat/interject 上报路径完成（server.go 的
// MarkVoiceUsed）。这里测快照拼装的时分——出现过的会话每轮都稳定带提示
//（粘性：不因后续轮次改打字而消失），这正是旧全局 5 分钟窗口机制所缺的。
func TestVoiceUsedEnvPrompt(t *testing.T) {
	a := &App{Cfg: &Config{Workspace: t.TempDir()}}

	clean := a.envPrompt(&SessionMeta{Mode: "task"})
	if strings.Contains(clean, "语音输入") {
		t.Fatalf("未用过语音的会话不该带语音提示:\n%s", clean)
	}

	used := a.envPrompt(&SessionMeta{Mode: "task", VoiceUsed: true})
	if !strings.Contains(used, "使用过语音输入") || !strings.Contains(used, "同音错字") {
		t.Fatalf("置位后的环境快照应含同音错字提示:\n%s", used)
	}

	// 粘性：同一状态每轮组装的快照逐字稳定——提示不闪变、不中途消失
	if again := a.envPrompt(&SessionMeta{Mode: "task", VoiceUsed: true}); again != used {
		t.Fatal("同一状态的环境快照应逐字稳定（粘性提示不该闪变）")
	}

	// 不串会话：别的会话（新 SessionMeta 未置位）不受某个会话置位影响
	if other := a.envPrompt(&SessionMeta{Mode: "office"}); strings.Contains(other, "语音输入") {
		t.Fatalf("未置位会话被串扰出了语音提示:\n%s", other)
	}
}
