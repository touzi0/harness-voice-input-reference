# 最好的 harness 语音识别机制参考

> ⚠️ **仅供参考，不能直接运行**（缺模型/运行时/宿主程序）。

这是一份**从我们自己的语音输入法组件（VoiceType）里摘出来的参考实现**，不是产品、不能直接运行、不保证完整，
用来看清三件事是怎么做的：

1. **音频记录**：每次说完留一条记录（文字 + 结果 + 原始音频），滚动只保**最近三条**，
   被挤出的音频一并删掉；
2. **语音识别**：从麦克风 PCM 到文字的完整调用链（实时切段 / VAD 分段 / 送进识别引擎 /
   拿回结果 / 超时收口）；
3. **语音输入的事件注入**（最关键的一块）：用户用了语音输入之后，
   **往模型的运行环境快照里加一句说明**——"这是语音转写，可能混有同音错字，
   请按上下文理解"。加了什么、加在哪个位置、怎么判断"这次是语音输入"、为什么
   不直接纠正错字，见 [`prompt-injection/`](prompt-injection/)。

> ⚠️ **仅供参考，不能直接运行。** 缺的东西是硬缺：没有模型文件（X-ASR 4 个 onnx +
> silero VAD，原组件里共约 168 MB）、没有随组件分发的便携 Python 运行时与
> `sherpa-onnx`/`numpy` 依赖、没有宿主程序（`prompt-injection/` 里的后端代码属于宿主，
> 这里只有片段）、也没有编译产物和 WebView2 前端页面。`go build` 也跑不起来
> （`go.mod` 的 `replace` 指向的 `go-webview2` 本地依赖没有随包附带）。
> 请把它当**读的材料**，不要当能跑的项目。

---

## 一、摘取方式：选了 B（整组件去掉模型/产物/前端），外加一块手工摘录

对照任务书里的两种做法：

- **B｜整个组件作为项目搬过来（去掉模型、产物、纯前端 UI）**：本项目主体采用这一种。
  去掉模型、exe、WebView2 用户数据目录、前端页面之后，剩下的 Go + Python 源码只有
  **约 223 KB / 16 个文件**（不含下面的片段摘录与文档），结构清楚、彼此引用关系完整
  （录制 → 识别 → 判定 → 输出 → 落档都在同一套 Go 包里），所以搬整套比只挑几个文件
  更能看出调用关系。
- **A｜只摘三块相关的源文件**：**第 3 块（提示词注入）必须用这一种**。因为那段逻辑在原产品里
  跨了「组件前端 → 宿主前端 → 宿主后端 → 快照拼装」四个位置，还横跨两个语言，
  没有办法用"搬文件"表达；而且宿主那几个文件分别是 4000+ 行 / 1500+ 行的巨型文件，
  整份搬过来既跑不了也看不清。所以这一块是**按数据流顺序手工摘出的 5 个片段文件**，
  每个文件头都写了原文路径与行号。

一句话：**主体用 B（整组件），第三块用 A（片段摘录）**，README 里两处都标了来源。

### 去掉了什么，为什么

| 去掉的东西 | 原体积 | 为什么去掉 |
|---|---|---|
| `models/xasr/*.onnx`（encoder 161 MB / decoder 11 MB / joiner 2.5 MB + tokens/bpe） | ≈ 168 MB | 模型权重，任务明确不要；且模型本身不是"实现" |
| `models/silero_vad.onnx` | 629 KB | 同上（VAD 模型） |
| `models/xasr/test_wavs/*.wav`、`test_onnx.py` | ≈ 0.9 MB | 模型自带的跑通脚本与测试音频，与本文三块逻辑无关 |
| `voicetype/voicetype_go.exe` | 6.7 MB | 打包产物 |
| `voicetype/webview2-profile/`（EBWebView 用户数据目录） | 数十 MB | 浏览器运行时缓存，纯垃圾，且含本机历史/书签 |
| `voicetype/go-webview2/`（含 3 个 `WebView2Loader.dll`） | ≈ 0.65 MB | 第三方库（他人 MIT 代码）+ 二进制 DLL，不是我们的实现 |
| `voicetype/history/*.wav`、`history.json` | 197 KB | 真实使用留下的录音与文字（用户数据），不搬；格式在下面「音频记录」一节给出 |
| `vt.log`、`asr.log` | 4 KB | 运行日志，含本机时间与路径 |
| `启动输入法.bat`、`voicetype_go.exe` 的启动脚本 | 126 B | 只为跑 exe，没有 exe 就没有意义 |
| `overlay.html`（悬浮窗胶囊 UI）、`client.css`、`client.js` 的界面部分 | 118 KB | **纯前端界面**（悬浮窗 + 设置页），任务允许只留目录说明；但 `client.js` 里**打语音标记**那一小段是第 3 块逻辑的起点，已摘进 `prompt-injection/01-component-mark.js` |
| 宿主程序的 `web/app-chat.js`、`server.go`、`store.go`、`agent.go`、`context.go` 等 | 数 MB | 宿主程序本体，与语音无关的部分一概不带；只摘与"语音标记/环境快照"相关的行，见 `prompt-injection/` |

---

## 二、目录与关键文件

```
语音输入法-参考实现/
├── README.md                     ← 本文件
├── voicetype/                    ← 独立语音输入法（Go + Python），B 方案整套搬入
│   ├── main.go                   主流程：拉起/保活 ASR 服务、调识别、超时收口、
│   │                             ★历史记录（最近三条）、会话收尾与输出、悬浮窗状态
│   ├── vt_windows.go             Windows 实现层：键盘/鼠标钩子与吞键规则、waveIn 采集、
│   │                             ★实时切段器（segStep/emitSeg）、麦克风选择、
│   │                             剪贴板 + Ctrl+V 注入、悬浮窗窗口管理、钩子看门狗
│   ├── vt_target_windows.go      「上屏还是只进剪贴板」的四层判定（vtEditabilityGate）
│   ├── vt_uia_windows.go         UIA：读焦点元素的 ValuePattern / TextPattern / 密码框
│   ├── vt_link_windows.go        与宿主程序联动：听写热键与主题色（voicetype.json）
│   ├── asr_server.py             ★本地识别服务（HTTP 127.0.0.1:8320）：
│   │                             silero VAD 分段 → 超长段能量谷切分 → 逐段增益 → X-ASR 识别
│   ├── go.mod / go.sum           Go 模块与依赖（replace 指向的 go-webview2 未附带）
│   ├── hotkeys.json              热键配置示例（键位/长按阈值/点按回放/避让宿主）
│   ├── vt_target_windows_test.go 判定逻辑 + ★历史滚动裁剪（TestHistoryWavAndRoll）等测试
│   ├── vt_hotkey_windows_test.go 吞键规则的纯逻辑测试
│   ├── vt_asr_timeout_test.go    识别总超时随语音时长线性（TestVtAsrTimeoutSec）
│   ├── .gitignore                原仓库的忽略清单（可看出哪些属于大件）
│   └── README.original.md        原实现留下的 README 存档（工程细节最全，
│                                 已脱敏开发机绝对路径；建议先读它再看代码）
├── component/                    ← 组件形态的入口（宿主把语音当"组件"加载时用）
│   ├── manifest.json             组件清单：声明 worker、启动命令、HTTP 端点
│   └── asr_worker.py             ★组件侧识别 worker：stdin/stdout 行 JSON 协议，
│                                 伪流式（停顿出灰字草稿，端点触发时整句精修定稿）
└── prompt-injection/             ← ★第三块：语音事件 → 提示词注入（A 方案，片段摘录）
    ├── README.md                 整条链的时序图、5 个片段的对应关系、为什么这么做
    ├── 01-component-mark.js      ① 组件侧：落框时打 dataset.jxVoice 标记
    ├── 02-send-report.js         ② 宿主前端：发送时读标记，上报 voiceUsed
    ├── 03-server-sticky-flag.go  ③ 宿主后端：置位会话粘性标记 + SQLite 列
    ├── 04-env-prompt-inject.go   ④ 宿主快照拼装：往里加那一行（**核心**）
    └── 05-voice-used_test.go     ⑤ 钉住这条行为的测试
```

> 目录里有两个"说法"要注意：`voicetype/` 是**独立进程版**（自己拉 Python 服务、自己注入文字），
> `component/` 是**组件版**（宿主拉起 worker、宿主负责往输入框里写）。两条链路共用同一套
> 识别内核与同一条"打标记 → 上报 → 注入提示词"的路径。

---

## 三、三块核心逻辑在哪、怎么串起来

### 1) 音频记录：滚动保留最近三条

**在哪**：`voicetype/main.go`

| 位置 | 作用 |
|---|---|
| `vtHistoryKeep = 3`（第 46 行） | 保留条数，唯一的旋钮 |
| `type vtHistoryItem`（第 354-359 行） | 一条记录：`time` / `text` / `status` / `audio` |
| `vtReadHistory()`（第 361 行） | 读 `history.json`（读不到就当空） |
| `vtSaveHistory(pcm, text, status)`（第 371 行） | **落一条 + 写 wav + 裁掉最老 + 删其音频** |
| `vtPCMToWav(pcm)`（第 397 行） | 给 16k/16bit/单声道裸 PCM 加 44 字节 WAV 头 |
| `TestHistoryWavAndRoll`（`vt_target_windows_test.go` 第 208 行） | 断言"只留 3 条、被挤出的 wav 真被删了" |

**存哪、怎么轮转、什么时候清**：

- **存哪**：`history.json`（一个小数组，按时间正序）与 `history/` 子目录下的 wav
  （文件名 `20261115-013049.720.wav`，毫秒精度避免同名覆盖）。两者都在程序自己的目录，
  不落 AppData、不落云、不外发。
- **怎么轮转**：`append` 之后，若 `len(arr) > vtHistoryKeep`，就**从数组头部**把多出来的
  条目逐个取出、`os.Remove` 掉它们指向的 wav，再 `arr = arr[len-vtHistoryKeep:]` 切片。
  即"追加 + 单向裁剪"，没有环形索引、没有时间判断——记录条数少到 3 条，简单可验，
  测试才敢直接断言"history/ 下还剩 3 个文件"。
- **什么时候清**：只在写新记录时顺带清（被挤出即删），没有定时清理、没有"打开设置页时清理"。
  一轮说完就写，写的时候顺手删。
- **为什么失败的轮次也留**：`status` 有四种（`injected` 已输入 / `copied` 已复制 /
  `asr_failed` 识别失败 / `empty` 没听清）。识别失败时**文字为空但音频照样落盘**——
  音频是"重新识别"的唯一原料（设置页的「重新识别」走 `vtReidentify()`，
  `main.go` 第 1185 行），说过的话不该因为某一环失败就丢。

**串起来**：`vtFinishSession()`（第 663 行）在识别失败 / 空结果 / 正常三条出口上都调
`vtSaveHistory`；正常出口经 `vtDeliverAfterAsr()`（第 647 行）拿到输出结果 status 再落一条。
所以"一轮说话 = 恰好一条记录"，不论成败。

`history.json` 的形状（`history/` 里的 wav 用相对路径引用，文件删了记录也随之裁掉）：

```json
[
  {
    "time":   "2026-11-15 01:30:49",
    "text":   "把上周的销售报表导出到桌面。",
    "status": "copied",
    "audio":  "history/20261115-013049.720.wav"
  }
]
```

`text` 为空 + `status` 为 `asr_failed`/`empty` 是正常形态（识别没成功，但音频在，
可以重新识别，也可以试听确认到底录到了什么）。

### 2) 语音识别：从音频到文字

两条链，同一套识别内核：

**A. 独立进程版（`voicetype/`）**

```
热键（vt_windows.go 的键盘/鼠标低级钩子 + 吞键规则）
  → vtStartCapture()            打开 waveIn，16k/mono/16bit，50ms 一帧
  → vtCapture.loop()            采集中；每帧把 RMS 喂给 segStep()
  → vtCapture.segStep()/emitSeg()   ★实时切段（伪流式）：能量门限状态机，
                                   语音≥800ms 且静音≥400ms → 从静音中点切一段，
                                   段长上限 15s，切点回退 200ms 余量防切掉字头
  → 中途段 → vtMidLoop() → vtTranscribe()（只为在悬浮窗上先出灰字）
  → 松开热键 → vtStopSession() → vtFinishSession()
  → vtTranscribeWithTimeout(pcm, vtAsrTimeoutSec(audioSec))   ★总超时 =
       语音时长 1:1 + 8s 缓冲、下限 15s、不设上限（长语音绝不提前打断）
  → vtTranscribe()（main.go 第 224 行）  16k PCM → 加 WAV 头 → multipart
       POST http://127.0.0.1:8320/v1/audio/transcriptions
  → asr_server.py：Handler.do_POST → transcribe_wav()
       → split_segments()        silero VAD 按 0.4s 静音断句
       → smart_hard_split()      超长段（>20s）回看 2.5s 找能量谷切，不硬切在字中间
       → normalize_peak()        ★逐段峰值归一（只提不降）；顺序纪律：
                                 切分必须在原始音频上跑，增益只能在切分之后逐段做
       → OfflineRecognizer（X-ASR transducer int8，sherpa-onnx）逐段解码
       → 拼成整段文字（英文/数字之间补空格）→ {"text": ...}
  → 回到 vtDeliverAfterAsr()：判定当前焦点可写吗 → 注入或只复制 → 落历史
```

要点：
- **识别服务的生命周期**：`startAsr()`（第 159 行）先探 `GET /health`，端口上已有服务就**复用**
  （主程序被强杀时 Python 子进程会变孤儿，复用比再拉一份省 ~500MB）；没有才 `exec`
  拉起 `asr_server.py`（`CREATE_NO_WINDOW`，输出重定向 `asr.log`）。`vtAsrWatch()`
  每 5 秒探活，连续 2 次失败才判死、重启带 20 秒冷却——"哪天 python 没了，
  说话就永远识别失败"这个坑就是它填的。
- **超时收口**：`vtTranscribeWithTimeout` 用 `deadline` 贯穿（含等服务就绪 + HTTP 重试），
  到点必返回"成功/失败/超时"三态，迟到结果直接丢弃（不会在"识别超时"之后再突然注入文字）。
- **为什么 VAD 要加锁**：silero VAD 与识别引擎都是共享的非线程安全实例，
  "中途段识别"和"松手后的最终全量识别"会并发打进来（`ThreadingHTTPServer`），
  只锁识别引擎不锁 VAD 会崩（`NULL input supplied for input h`）——`asr_server.py`
  的 `transcribe_wav()` 里用一把 `_lock` 把"分段 + 逐段识别"整体串起来。

**B. 组件版（`component/`）**：`asr_worker.py` 不监听端口，走 stdin/stdout 行 JSON：

```
{"cmd":"load"}  → 加载 X-ASR
{"cmd":"stream_start"} → {"cmd":"stream_chunk", pcm_b64}  ← 宿主每 50ms 喂一块
    停顿（端点）时回 {"event":"final", text:"整句（离线精修，落黑字）"}
    期间可回 {"event":"partial", text:"灰字增量"}
{"cmd":"stream_end"} → {"event":"done"}
{"cmd":"transcribe", wav_b64} → {"text": ...}（一次性整段识别）
```

同一套内核函数在这里叫 `_get_vad` / `_split_segments` / `_smart_hard_split` / `_normalize_gain`，
逻辑与 `asr_server.py` 一致（前者是组件形态，后者是独立进程形态）。

**C. 还有第三条链（没搬）**：宿主的"浏览器听写"走 WebView2 的 Web Speech（云端识别），
代码在 `组件/voice/client.js` 的 `bindVoiceButton()` 一带，属纯前端 UI，按任务要求只留说明。

### 3) 语音输入 → 往提示词里注入（最关键的一块）

全部在 [`prompt-injection/`](prompt-injection/)，按数据流 5 个文件：

| 步骤 | 文件 | 原文函数 | 干了什么 |
|---|---|---|---|
| ① | `01-component-mark.js` | `commitText()` | 语音文字落进输入框的同时 `inp.dataset.jxVoice = '1'` |
| ② | `02-send-report.js` | `prepareSend()` / `send()` / `sendMidrun()` | 发送时 `voiceUsed = fromInput && input.dataset.jxVoice === '1'`，随消息上报，随即删标记 |
| ③ | `03-server-sticky-flag.go` | `handleChat()` / `handleInterject()` / `MarkVoiceUsed()` | 收到 `voiceUsed` 就 `m.VoiceUsed = true` + `UPDATE sessions SET voice_used=1`（单调置位） |
| ④ | `04-env-prompt-inject.go` | `envPrompt()` 第 686-688 行 | `if sess.VoiceUsed { b.WriteString("- 用户在本对话中使用过语音输入…") }` |
| ⑤ | `05-voice-used_test.go` | `TestVoiceUsedEnvPrompt` | 钉住：未用过不出现 / 用过每轮都在 / 逐字稳定 / 不串会话 |

**注入的那句话（原文，一个字没改）**：

```
- 用户在本对话中使用过语音输入（听写转写）：消息可能混有同音错字，请按上下文理解其本意，
  在回复中自然使用正确写法，不必特意指出
```

**加在提示词的哪个位置**：

- 不是加进**系统提示词**（那是稳定前缀，逐字不变才能吃满前缀缓存）；
- 而是加进**「运行环境快照」**——一条独立的 `role=system` 消息，由 `envPrompt()` 拼装、
  由 `maybeAppendEnvSnapshot()` 追加到对话历史尾部（`Kind = kindEnv`），
  首次整份下发，之后**按行差分**只发变化行（纯新增发短补充，有减/换值发【移除】/【新增】清单）。
- 在 `envPrompt()` 内部，它位于"逐会话的长期状态行"那一段（操作系统 / 工作区 /
  权限模式 / 工具能力 / 计划模式 / 目标模式 / 子对话 **← 语音这一行 →** 插件动态段 /
  用户记忆块 / 组件能力块 / 技能清单）。
- 结果是：模型第 0 轮就看见这句话，但**前缀一个字节没动**，这段注入的代价是
  一次性约 40 token，且因为粘性 + 逐字稳定，之后每轮组装结果完全相同，不产生任何增量。

**怎么判断"这次是语音输入"**：不靠猜文本、不靠启发式，是一条显式的链路——
组件是**唯一**知道"这段文字是识别出来的"的一方，它写输入框时打一个 `dataset` 标记
（同一处也是删标记的地方：发送清框、用户手动删光都会删）；宿主前端发送时读这个标记，
作为 `voiceUsed` 上报；宿主后端把它落成**会话级粘性标记**（一次置位、终身有效）。
后端从不重新判断（它只收到一段和手打无异的文本）。

**为什么不直接纠正错字，而是交给模型按上下文理解**（详见 `prompt-injection/README.md`）：

- 纠错层只有读音相似性，没有句子意图、没有对话历史、没有领域词汇表；同音字在字面上
  通常**没有局部正确答案**（"接口/借口"），猜错就是悄悄篡改用户原话——比不改更糟，
  因为下游模型再没有任何信号提示这段文本可疑。
- 语音错字经常成串出现（人名、型号、专有名词），纠错层一错错一串，很容易把本来对的词改坏。
- 模型同时看得到这句话、前几轮对话、工作区里的文件与用户历史偏好——给它"这是语音转写、
  可能有同音错字"这一句元信息，信息量严格大于给它一段被粗暴改写过的文本。
- 转写是证据、纠错是解释：两层分开，出问题还能回查用户到底说了什么
  （配合上面"最近三条原始音频"的记录）。
- 成本上：一行 prompt ≈ 40 token、一次性、可缓存；专门跑一次纠错模型调用通常更贵，
  且多一个失败点。

所以分工是：**识别层负责"听清楚"，提示词层负责告诉模型"这段人话是听来的"。**

---

## 四、原路径 → 新路径对照表

| 原路径（`D:\智能体工作区\软著版数字助理\组件\voice\` 下） | 新路径（本项目内） | 这个文件是干什么的 / 为什么留 |
|---|---|---|
| `voicetype/main.go` | `voicetype/main.go` | 主流程：ASR 服务拉起与保活、识别调用与超时、**历史三条**、会话收尾、输出分发、悬浮窗状态、自测触发器。三块核心逻辑有两块半在这里 |
| `voicetype/vt_windows.go` | `voicetype/vt_windows.go` | Windows 实现层：钩子与吞键、waveIn 采集、**实时切段器**、剪贴板/注入、窗口管理。录音链路的主体 |
| `voicetype/vt_target_windows.go` | 同 | "上屏还是只进剪贴板"的四层判定（`vtEditabilityGate`）。识别完往哪儿送，靠它 |
| `voicetype/vt_uia_windows.go` | 同 | UIA 读取焦点元素（ValuePattern/TextPattern/密码框）。判定第 3 层的实现 |
| `voicetype/vt_link_windows.go` | 同 | 与宿主联动（听写热键、主题色）。说明"输入法是独立进程时怎么和宿主对齐配置" |
| `voicetype/asr_server.py` | 同 | **识别服务**：VAD 分段、能量谷切分、逐段增益、X-ASR 解码、HTTP 端点。第 2 块核心 |
| `voicetype/go.mod`、`go.sum` | 同 | 模块与依赖清单（含 `replace ./go-webview2`，该目录未附带） |
| `voicetype/hotkeys.json` | 同 | 热键配置示例（键位/长按阈值/点按回放/避让宿主名单） |
| `voicetype/vt_target_windows_test.go` | 同 | 判定逻辑测试 + **`TestHistoryWavAndRoll`（三条滚动 + 删音频）** |
| `voicetype/vt_hotkey_windows_test.go` | 同 | 吞键规则纯逻辑测试（为什么"按住 Alt 说话"不会激活菜单栏） |
| `voicetype/vt_asr_timeout_test.go` | 同 | 识别总超时随语音时长线性递增 |
| `voicetype/.gitignore` | 同 | 原仓库忽略清单，可看出哪些属于大件（模型/产物/profile） |
| `voicetype/README.md` | `voicetype/README.original.md` | **原实现的 README 存档**（1.7 万字符，吞键/判定/历史/超时/编译全在里面）。已脱敏两条开发机绝对路径 |
| `manifest.json` | `component/manifest.json` | 组件清单：声明 worker、启动命令 `{python} asr_worker.py`、端点 `POST asr/transcribe` |
| `asr_worker.py` | `component/asr_worker.py` | **组件侧识别 worker**（stdin/stdout 行 JSON、伪流式草稿/定稿）。第 2 块核心的另一种形态 |
| `client.js` 的 `voiceJoin()` / `commitText()` / 标记生命周期（第 784-794、866-877、1782-1791 行） | `prompt-injection/01-component-mark.js` | 第 3 块逻辑的**起点**：打语音标记。原文件 97 KB，其余部分是 UI，未搬 |
| `web/app-chat.js`（第 5882-5888、5914-5917、5966-5970、6049-6051、6098-6101 行） | `prompt-injection/02-send-report.js` | 宿主前端读标记、上报 `voiceUsed` |
| `server.go`（第 2996-3006、3991-4008 行）、`store.go`（第 327-336、712、716-721、782-790、809 行） | `prompt-injection/03-server-sticky-flag.go` | 后端置位粘性标记、SQLite 列与单一写入口 |
| `agent.go`（第 511-522、683-690、983-1014 行）、`kinds.go`（第 22 行）、`contextengine.go`（第 80 行） | `prompt-injection/04-env-prompt-inject.go` | **注入本身**：环境快照拼装 + 注入的那一行 + 落进上下文的方式 |
| `voice_used_test.go`（整文件） | `prompt-injection/05-voice-used_test.go` | 钉住这条行为的测试 |

**删掉了哪些、为什么删**（同样见第一节的表格）：模型与 VAD 权重（168 MB，任务明确不要）、
`test_wavs` + `test_onnx.py`（模型自测材料，与实现无关）、`voicetype_go.exe`（打包产物）、
`webview2-profile/`（浏览器缓存，数十 MB，含本机痕迹）、`go-webview2/` 与 3 个
`WebView2Loader.dll`（第三方库 + 二进制）、`history/` 与 `history.json`（真实录音与文字，
用户数据）、`vt.log`/`asr.log`（运行日志）、`启动输入法.bat`（只为跑 exe）、
`overlay.html`/`client.css`/`client.js` 的界面部分（纯前端 UI，任务允许只留说明；
其中打标记的几行已摘出）。

**脱敏改动**（都在文件里留了注释，便于核对）：

| 位置 | 改成什么 |
|---|---|
| `voicetype/main.go` `findPython()` 的两条候选 | 删掉两条含**开发机用户名**的绝对路径（`D:\...\用户目录\...`），换成一行说明注释 |
| `voicetype/vt_link_windows.go` `vtLinkCandidatePaths()` | 同上，删掉一条开发机绝对路径兜底 |
| `voicetype/asr_server.py` `_resolve_model_dir()` | 删掉一条指向内部仓库目录名的模型候选路径，并同步改注释 |
| `voicetype/README.original.md` | 两处开发机绝对路径换成脱敏说明 |

其余内容未做任何改写：`127.0.0.1:8320` 是本地回环端口（不是内网地址），
`pypi.tuna.tsinghua.edu.cn` 是公开镜像，`坚祥数字助理GO.exe` 等是"避让宿主"的程序名
（功能性配置，不含凭据）。

---

## 五、自检

- **凭据与地址**：无 API Key、无 token、无 SSH 口令、无内网地址（无 `192.168.x`）、
  无公司域名、无真实用户名/手机号。`127.0.0.1` 仅为本地回环；
  密码相关字样只有 `IsPassword`/`ValuePattern` 这类 UIA 字段名。
- **二进制与模型**：目录内**没有任何 exe/dll/onnx/wav/zip**，全部是文本源码与配置。
- **体积**：**23 个文件、共 302,098 字节（295.0 KB）**，明细见下；
  原组件目录 194.3 MB，去掉的 194 MB 全是模型/产物/缓存/用户数据。
- **文本性**：23 个文件全部是 `.go/.py/.js/.json/.md/.gitignore` 纯文本，
  没有 exe/dll/onnx/wav/zip 任何一种。
- **不能运行**：见开头声明——缺模型、缺运行时依赖、缺宿主、缺前端页面、
  缺 `go-webview2` 本地依赖。
- **文件明细**（字节）：

```
README.md                        26,561   ← 本文件
component/asr_worker.py          16,394
component/manifest.json             836
voicetype/main.go                57,178
voicetype/vt_windows.go          70,152
voicetype/vt_target_windows.go   12,264
voicetype/vt_uia_windows.go       8,980
voicetype/vt_link_windows.go      7,736
voicetype/asr_server.py          15,743
voicetype/go.mod                    311
voicetype/go.sum                    545
voicetype/hotkeys.json              326
voicetype/.gitignore                903
voicetype/README.original.md     17,680
voicetype/vt_target_windows_test.go   14,632
voicetype/vt_hotkey_windows_test.go   21,139
voicetype/vt_asr_timeout_test.go       1,558
prompt-injection/README.md         5,383
prompt-injection/01-component-mark.js  3,708
prompt-injection/02-send-report.js     3,654
prompt-injection/03-server-sticky-flag.go  5,249
prompt-injection/04-env-prompt-inject.go   8,065
prompt-injection/05-voice-used_test.go     2,674
                                 ─────────
                        合计     302,098 字节 = 295.0 KB
```

---

## 六、这种做法对我们 harness 的意义

语音输入真正的难点不在"把声音转成字"，而在**让模型知道这段字是听来的**——
给输入带上来源与不确定性的上下文，比让模型自己从错字里硬猜、或者换一个更大的模型去硬纠错，
成本低得多、也更可靠。
