/* ============================================================================
 * ② 宿主前端：发送消息时读出标记，把它作为 voiceUsed 随消息上报
 * ----------------------------------------------------------------------------
 * 原文：web/app-chat.js（宿主程序的前端，不是语音组件的一部分）
 *   第 5882-5888 行  sendMidrun()（执行中插话/排队的发送口）
 *   第 5914-5917 行  插话路径发送成功后的清标记
 *   第 5966-5970 行  prepareSend()：判断"这条是不是语音输入"
 *   第 6049-6051 行  正常发送路径的清标记
 *   第 6098-6101 行  send()：把 voiceUsed 带进 POST /api/sessions/{id}/chat
 *
 * 上下文：所有发送路径（回车 / Ctrl+回车 / 发送按钮 / 子对话 / 执行中插话）
 *   都汇到 prepareSend()，标记只在那里读一次，再把 voiceUsed 传下去。
 * ========================================================================== */

/* ---- 判断"这条是不是语音输入"（唯一判据）------------------------------- */

  await JX.awaitHooks('send', { view });
  const input = view.input;
  const fromInput = typeof forceContent !== 'string';
  // 本条消息含语音听写转写文字（组件听写落框时在输入框上打的标记 jxVoice）：
  // 随消息上报后端置位会话粘性标记，环境快照自此每轮带同音错字提示（server.go）
  const voiceUsed = fromInput && input.dataset.jxVoice === '1';

  /* 判据解读：
   *   fromInput —— 只有"从输入框发出去"的才算；编辑重发/降级补发（forceContent
   *                路径）内容已经完整，不再叠加语音语义。
   *   dataset.jxVoice === '1' —— 严格等于 '1'，不做真值判断（避免 '0'/'false' 也命中）。
   */

/* ---- 正常发送：voiceUsed 进 POST body，随即清标记 ---------------------- */

  try {
    const res = await fetch(`/api/sessions/${sid}/chat`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ content, voiceUsed: !!pre.voiceUsed }),
      signal: view.state.abort.signal,
    });

  if (fromInput) {
    input.value = ''; clearQuote(view); // 引用/附件已组装进这条消息：输入框与胶囊一起退场
    delete input.dataset.jxVoice; // 语音标记已随本条消息上报，随之退场
    autoGrow(input);

/* ---- 执行中插话/排队：同一个 voiceUsed 也带过去 ------------------------ */

async function sendMidrun(content, viaInput, view, voiceUsed) {
  view = view || mainView;
  const mode = lsGet('jx.midrun', 'queue') === 'interject' ? 'steer' : 'queue';
  try {
    const d = await api('/api/sessions/' + view.state.cur.id + '/interject', {
      method: 'POST', body: JSON.stringify({ content, mode, voiceUsed: !!voiceUsed }),
    });
    if (!d.accepted) { pendingHint(view, '发送失败：该会话当前不接受消息'); return; }

  if (viaInput) {
    view.input.value = ''; clearQuote(view); autoGrow(view.input);
    delete view.input.dataset.jxVoice; // 语音标记已随本条消息上报，随之退场

/* ============================================================================
 * 标记为什么在发送时就删掉：
 *   标记的语义是"这条即将发出的消息里有语音转写"，发出去就完成了使命。
 *   留着它会让下一条纯打字的消息也带上 voiceUsed（虽然服务端标记已经是粘性的，
 *   多报一次无害，但前端保持"标记只描述当前这一条"更不容易出错）。
 * ========================================================================== */
