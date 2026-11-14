/* ============================================================================
 * ① 组件侧：语音文字落进输入框时，给输入框打一个「这条含语音转写」的标记
 * ----------------------------------------------------------------------------
 * 原文：组件/voice/client.js
 *   第 784-794 行  voiceJoin()
 *   第 866-877 行  commitText()
 *   第 1782-1791 行 标记生命周期（init 里挂的监听）
 *
 * 上下文：本组件的听写（浏览器 SpeechRecognition）先说出的文字是「灰字草稿」，
 *   不在输入框的值里；静置 2.5s（VOICE_SETTLE_MS）或用户按停后才由 commitText()
 *   落成黑色正文。落框是全组件唯一写输入框正文的入口，所以标记只在这里打一次。
 *
 * 注意：这里只摘与「打标记」有关的代码，灰字叠层、重连退避、倒计时等纯 UI 逻辑
 *   没有搬（见主 README「删掉了什么」）。
 * ========================================================================== */

/* ---- 拼装上一段已有文字与新到文字（中文之间不加空格）-------------------- */

  /**
   * 把「上一段已有文字」与「新到文字」拼起来：中文之间不加空格（语音转写的中文
   * 分段常常只是停顿，插空格会切碎句子），英文/数字单词之间保留一个空格。
   */
  function voiceJoin(prev, next) {
    if (prev === '') return next;
    const tail = prev.charAt(prev.length - 1), head = next.charAt(0);
    return (/[A-Za-z0-9]/.test(tail) && /[A-Za-z0-9]/.test(head)) ? prev + ' ' + next : prev + next;
  }

/* ---- 落框 + 打标记（整条链的起点）-------------------------------------- */

  /** 把一段文字落进输入框正文（程序化写入不触发 input 事件：手动落草稿）。
   *  落框顺带打 jxVoice 标记：发送时 app-chat.js 读它随消息上报 voiceUsed，
   *  后端置位会话粘性标记——用过一次语音，环境快照从此每轮带同音错字提示。
   *  输入框被清空（发送清框 / 用户手动删光）时标记随之撤销（见 init 的监听）。 */
  function commitText(view, text) {
    if (!text) return;
    const inp = view.input;
    inp.value = voiceJoin(inp.value, text);
    inp.dataset.jxVoice = '1';
    autoGrow(inp);
    if (view === mainView) persistMainDraftLS(); else persistSubDraftLS();
  }

/* ---- 标记的撤销：用户手动删光输入框时也不该再算语音 -------------------- */

      // 语音标记生命周期：输入框被清空时撤销 jxVoice（发送清框在 send 链路里
      // 已删；这里兜用户手动删光的场景）——之后纯打字的消息不会被误标为语音。
      // jxVoiceBound 防重挂（组件关开重载 init 会再跑一遍）。
      for (const view of [mainView, subView]) {
        const inp = view.input;
        if (!inp || inp.dataset.jxVoiceBound) continue;
        inp.dataset.jxVoiceBound = '1';
        inp.addEventListener('input', () => { if (!inp.value) delete inp.dataset.jxVoice; });
      }

/* ============================================================================
 * 为什么标记打在 dataset 而不是放进消息内容里：
 *   输入框里的文字是用户还能改的（落框后与手打文字一样可编辑）。用户可能
 *   落框之后又补一句手打的话，也可能把语音那段删掉只留手打——所以"这条消息
 *   是否含语音"只能按"输入框被清空即撤销"来近似，不能把标记固化进正文，
 *   否则用户在输入框里改到面目全非，标记还在。
 * ========================================================================== */
