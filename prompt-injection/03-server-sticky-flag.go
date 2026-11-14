// ============================================================================
// ③ 宿主后端：收到 voiceUsed 就置位「本会话用过语音」的粘性标记
// ----------------------------------------------------------------------------
// 原文：server.go（HTTP 路由处理）、store.go（SQLite 列与读写）
//   server.go 第 3991-4009 行  handleChat()       —— 正常发送
//   server.go 第 2996-3006 行  handleInterject()  —— 执行中插话/排队（同一个标记）
//   store.go  第 327-336 行    建表迁移：voice_used 列
//   store.go  第 712 行        sessionCols（读列清单）
//   store.go  第 716-721 行    SaveMeta 的注释：这一列归 MarkVoiceUsed 独占
//   store.go  第 782-790 行    MarkVoiceUsed()
//   store.go  第 808-810 行    scanSession：列 → SessionMeta.VoiceUsed
//
// 说明：以下是 Go 片段，不能单独编译（它们依赖宿主的 App / Store 类型）。
// ============================================================================

// ---- 正常发送路径 ----------------------------------------------------------

func (a *App) handleChat(w http.ResponseWriter, r *http.Request) {
	m := a.sessionFromPath(w, r)
	if m == nil {
		return
	}
	var body struct {
		Content   string `json:"content"`
		VoiceUsed bool   `json:"voiceUsed"` // 本条消息含语音听写转写文字（组件听写落框时打的标记）
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Content) == "" {
		writeErr(w, 400, "content 不能为空")
		return
	}
	// 语音听写粘性标记：会话内用过一次即置位且永不撤下——环境快照（agent.go）
	// 据此注入同音错字提示；内存对象同步置位，本轮组装快照时即已生效。
	if body.VoiceUsed && !m.VoiceUsed {
		m.VoiceUsed = true
		_ = a.Store.MarkVoiceUsed(m.ID)
	}

// ---- 执行中插话 / 排队路径（同一个标记，两个入口都要处理）-----------------

	var body struct {
		Content   string `json:"content"`
		Mode      string `json:"mode"`
		VoiceUsed bool   `json:"voiceUsed"` // 本条消息含语音听写转写文字：置位会话粘性标记
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Content) == "" {
		writeErr(w, 400, "content 不能为空")
		return
	}
	// 语音听写粘性标记：用过一次即置位且永不撤下（环境快照据此注入同音错字提示）
	if body.VoiceUsed && !m.VoiceUsed {
		m.VoiceUsed = true
		_ = a.Store.MarkVoiceUsed(m.ID)
	}

// ---- 持久化：一列，默认 0 --------------------------------------------------

	{
		// 会话内用过语音听写（粘性标记）：前端随 chat 请求上报 voiceUsed
		//（组件听写落框时打标记），置位后永久保持——环境快照据此稳定注入
		// 同音错字提示（agent.go），不因后续轮次改打字而撤下。
		id:       "0015_session_voice_used",
		tolerant: true,
		sql: []string{
			`ALTER TABLE sessions ADD COLUMN voice_used INTEGER NOT NULL DEFAULT 0`,
		},
	},

const sessionCols = `id, title, mode, workspace, parent_id, provider, model, permission, plan_mode, goal_mode, tools, remote, archived, unread, status, created_at, updated_at, thinking, goal_text, goal_status, goal_rounds, usage_prompt, usage_cache_hit, usage_output, usage_calls, origin, dispatched_by, expert, voice_used`

func (s *Store) SaveMeta(m *SessionMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 注意：下面的 UPDATE 刻意不写 status——它只由 UpdateStatus 维护（执行中/空闲）；
	// 同理不写 voice_used——它只由 MarkVoiceUsed 单调置位（chat 上报路径），陈旧
	// 内存对象全列回写不得把它冲回去。

// MarkVoiceUsed 置位会话的语音听写粘性标记（用过一次即永久保持）。列所有权
// 归本函数独占：SaveMeta 的 UPDATE 不回写该列（同 status 之于 UpdateStatus 的
// 先例），陈旧内存对象的全列回写不会把已置位的标记冲掉。
func (s *Store) MarkVoiceUsed(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE sessions SET voice_used=1 WHERE id=?`, id)
	return err
}

	// scanSession 里：
	m.VoiceUsed = vu == 1

// ============================================================================
// 为什么是「单调置位」而不是「每轮按当前消息重算」：
//   1) 提示词要稳定。环境快照是差分下发的（见 ④），同一状态下每轮必须组装出
//      逐字相同的快照，否则每轮都触发一次「环境更新」。若按"这一条是不是语音"
//      逐轮重算，用户一会儿说话一会儿打字，快照就会反复增删那一行。
//   2) 语义上也对：这个提示是给模型的一句**长期背景**（"这个用户会用语音跟我
//      说话，看到同音字别当真的错别字"），不是对某一条消息的批注。
//   3) 单一写入口。列所有权归 MarkVoiceUsed，SaveMeta 的全列回写刻意跳过它，
//      否则并发下陈旧内存对象会把已经置位 1 冲回 0（与 status 列同一个坑）。
// ============================================================================
