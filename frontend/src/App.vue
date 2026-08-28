<template>
  <div class="app">
    <!-- 无边框窗口标题栏 -->
    <div class="titlebar" style="--wails-draggable: drag;">
      <div class="titlebar-title">
        <span class="dot" style="--wails-draggable: none;"></span>
        Onekey
      </div>
      <div class="titlebar-controls" style="--wails-draggable: none;">
        <button class="tb-btn" @click="WindowMinimise">&#x2013;</button>
        <button class="tb-btn" @click="WindowToggleMaximise">&#9633;</button>
        <button class="tb-btn tb-close" @click="Quit">&#10005;</button>
      </div>
    </div>

    <!-- 顶部状态条 -->
    <div class="statusbar">
      <span class="status-item" :class="{ ok: steamOk }">
        <i class="status-dot" :class="{ ok: steamOk }"></i>
        {{ steamOk ? 'Steam 已就绪' : '未检测到 Steam 路径' }}
      </span>
      <span class="status-item" :class="taskClass">
        <i class="status-dot" :class="taskClass"></i>
        {{ taskLabel }}
      </span>
      <span v-if="taskMsg" class="status-item task-msg">{{ taskMsg }}</span>
    </div>

    <main class="content">
      <!-- 设置 -->
      <section class="card">
        <h2>设置</h2>
        <div class="grid-2">
          <label class="field">
            <span>Steam 路径</span>
            <input v-model="steamPath" placeholder="自动检测，可手动覆盖"/>
          </label>
          <label class="field">
            <span>代理地址</span>
            <input v-model="proxyUrl" placeholder="如 http://127.0.0.1:7890（留空为不使用）"/>
          </label>
        </div>
        <div class="row">
          <button class="primary" :disabled="saving" @click="saveConfig">{{ saving ? '保存中…' : '保存设置' }}</button>
          <button :disabled="testingProxy" @click="testProxy">{{ testingProxy ? '测试中…' : '测试代理' }}</button>
          <button class="ghost" @click="resetConfig">重置配置</button>
        </div>
      </section>

      <!-- 内核设置 -->
      <section class="card">
        <h2>内核设置</h2>
        <div class="switches">
          <label class="switch">
            <input type="checkbox" v-model="kernel.activate_unlock_mode" @change="saveKernel"/>
            <span class="track"><span class="thumb"></span></span>
            <span>激活解锁模式</span>
          </label>
          <label class="switch">
            <input type="checkbox" v-model="kernel.always_stay_unlocked" @change="saveKernel"/>
            <span class="track"><span class="thumb"></span></span>
            <span>常驻解锁</span>
          </label>
          <label class="switch">
            <input type="checkbox" v-model="kernel.not_unlock_depot" @change="saveKernel"/>
            <span class="track"><span class="thumb"></span></span>
            <span>不解锁仓库（depot）</span>
          </label>
        </div>
      </section>

      <!-- 搜索 / 解锁 -->
      <section class="card">
        <h2>搜索 / 解锁游戏</h2>
        <div class="row">
          <input v-model="searchTerm" placeholder="输入游戏名称，回车搜索" @keyup.enter="doSearch"/>
          <button :disabled="searching" @click="doSearch">{{ searching ? '搜索中…' : '搜索' }}</button>
        </div>
        <!-- AppID 直解锁：不经商店搜索，走国内 CDN 拉取解锁数据，商店域名断连时仍可用 -->
        <div class="row" style="margin-top: 0.6rem">
          <input v-model="appidTerm" placeholder="或直接输入 AppID 解锁（如 1144200）" @keyup.enter="unlockByAppId"/>
          <button class="primary" :disabled="unlocking" @click="unlockByAppId">{{ unlocking ? '解锁中…' : '直解锁' }}</button>
        </div>
        <div v-if="results.length" class="results">
          <div v-for="r in results" :key="r.id" class="item">
            <img v-if="r.tiny_image" :src="r.tiny_image" alt=""/>
            <div class="info">
              <div class="name">{{ r.name }}</div>
              <div class="meta">
                {{ r.type || 'app' }} · {{ formatPrice(r) }}
                <span v-if="r.platforms" class="platforms">
                  <span v-if="r.platforms.windows">Win</span><span v-if="r.platforms.mac">/Mac</span><span v-if="r.platforms.linux">/Linux</span>
                </span>
              </div>
            </div>
            <div class="actions">
              <button class="primary" @click="unlock(r)">解锁</button>
              <button @click="addToLibrary(r)">加入库</button>
            </div>
          </div>
        </div>
      </section>

      <!-- 已解锁游戏库 -->
      <section class="card">
        <h2>已解锁游戏库</h2>
        <div v-if="!library.length" class="empty">暂无解锁记录</div>
        <div v-for="g in library" :key="g.app_id" class="lib-item">
          <img v-if="g.tiny_image" :src="g.tiny_image" alt=""/>
          <div class="info">
            <div class="name">
              {{ g.name }}
              <em v-if="g.unlocked" class="badge">已解锁</em>
            </div>
            <div class="meta">AppID {{ g.app_id }} · {{ g.depot_count }} 仓库 · {{ g.dlc_count }} DLC</div>
          </div>
          <div class="actions">
            <button @click="toggleDetail(g.app_id)">{{ detailOpen === g.app_id ? '收起' : '详情' }}</button>
            <button class="danger" @click="removeGame(g.app_id)">移除</button>
          </div>
          <!-- 展开的详情 -->
          <div v-if="detailOpen === g.app_id && detail" class="detail">
            <div class="detail-block" v-if="detail.depots && detail.depots.length">
              <div class="detail-h">仓库</div>
              <div v-for="d in detail.depots" :key="d.depot_id" class="detail-row mono">
                <span>depot {{ d.depot_id }}</span>
                <span class="dim">key {{ d.depot_key ? d.depot_key.slice(0, 10) + '…' : '无' }}</span>
                <span class="dim">{{ d.manifest_id }}</span>
              </div>
            </div>
            <div class="detail-block" v-if="detail.dlcs && detail.dlcs.length">
              <div class="detail-h">DLC</div>
              <div v-for="d in detail.dlcs" :key="d.depot_id" class="detail-row mono">
                <span>app {{ d.app_id }}</span>
                <span class="dim">depot {{ d.depot_id }}</span>
                <span class="dim">{{ d.manifest_id }}</span>
              </div>
            </div>
            <div class="detail-note" v-if="detail.lua_path">Lua：{{ detail.lua_path }}</div>
          </div>
        </div>
      </section>

      <!-- 内核工具 -->
      <section class="card">
        <h2>内核工具</h2>
        <div class="row">
          <button class="primary" @click="installKernel">安装内核(OpenSteamTools)</button>
          <button class="primary" @click="restartSteam">重启 Steam</button>
        </div>
        <div class="kernel-state" :class="kernelState.ready ? 'ok' : 'err'">
          <i class="status-dot" :class="kernelState.ready ? 'ok' : 'error'"></i>
          内核状态：{{ kernelState.message }}
        </div>
        <div class="hint">内核已内置（免下载），安装后重启 Steam 一次即可生效；Lua 配置热重载，无需每次重启。</div>
        <details class="av-guide">
          <summary>杀软误报怎么办？（卡巴斯基 / Defender 等）</summary>
          <div class="av-guide-body">
            <p>内核是进程注入式代理，卡巴斯基、Windows Defender 等会把加载它的 Steam 判为"可疑活动"并杀掉/拦截。请在你的杀软里对 Steam 所在目录手动加白名单，即可稳定使用。</p>
            <p class="av-tip">Windows Defender：设置 → 病毒和威胁防护 → 排除项 → 添加排除 → 文件夹 → 选 Steam 目录，并选中 <code>dwmapi.dll / OpenSteamTool.dll / xinput1_4.dll</code>。</p>
            <p class="av-tip">卡巴斯基：设置 → 威胁与排除 → 信任区域/排除对象 → 添加 → 选 Steam 目录和上述 DLL（尤其勾选"系统监控"）。</p>
            <p class="av-tip">请勿让本程序或以提权方式自动修改系统防御设置——主动防御会判定为可疑终止程序。务必手动在你的杀软界面里操作。</p>
          </div>
        </details>
      </section>

      <!-- 运行日志 -->
      <section class="card">
        <h2>运行日志</h2>
        <div v-if="!logs.length" class="empty">暂无日志</div>
        <div v-for="(l, i) in logs" :key="i" class="log-line" :class="l.type">
          <span class="time">{{ l.timestamp }}</span>
          {{ l.message }}
        </div>
      </section>
    </main>
  </div>
</template>

<script lang="ts" setup>
import {computed, onMounted, ref} from 'vue'
import {Quit, WindowMinimise, WindowToggleMaximise, EventsOn} from '../wailsjs/runtime/runtime'
import {
  AddToLibrary,
  GetConfig,
  GetDetailedConfig,
  GetGameDetail,
  GetKernelSettings,
  GetLibrary,
  GetTaskStatus,
  KernelStatus,
  LoadKernel,
  RemoveFromLibrary,
  ResetConfig,
  RestartSteam,
  SearchStore,
  SetKernelSettings,
  StartUnlock,
  TestProxy,
  UpdateConfig,
} from '../wailsjs/go/main/App'

const searchTerm = ref('')
const searching = ref(false)
const results = ref<any[]>([])
const appidTerm = ref('')
const unlocking = ref(false)
const library = ref<any[]>([])
const configData = ref({steam_path: '', debug_mode: false})

const steamPath = ref('')
const proxyUrl = ref('')
const saving = ref(false)
const testingProxy = ref(false)

const kernel = ref({activate_unlock_mode: false, always_stay_unlocked: false, not_unlock_depot: false})
const kernelState = ref({ready: false, message: '检测中…'})

const taskStatus = ref('idle')
const taskResult = ref<any>(null)
const detailOpen = ref<number | null>(null)
const detail = ref<any>(null)

const logs = ref<Array<{type: string; message: string; timestamp: string}>>([])

const steamOk = computed(() => !!configData.value.steam_path)
const taskClass = computed(() => {
  const s = taskStatus.value
  if (s === 'running') return 'running'
  if (s === 'error') return 'error'
  if (s === 'completed') return 'ok'
  return 'idle'
})
const taskLabel = computed(() => {
  const map: Record<string, string> = {idle: '空闲', running: '处理中…', completed: '已完成', error: '出错'}
  return map[taskStatus.value] || taskStatus.value
})
const taskMsg = computed(() => {
  if (taskStatus.value === 'error' && taskResult.value) return taskResult.value.message
  if (taskStatus.value === 'completed' && taskResult.value) return taskResult.value.message
  return ''
})

function addLog(type: string, message: string) {
  logs.value.push({type, message, timestamp: new Date().toLocaleTimeString()})
  // 自动滚到底
  requestAnimationFrame(() => {
    const el = document.querySelector('.content')
    if (el) el.scrollTop = el.scrollHeight
  })
}

async function refreshConfig() {
  try {
    const r = await GetConfig()
    if (r.success) configData.value = r.config
  } catch {
  }
  try {
    const d = await GetDetailedConfig()
    if (d.success && d.config) {
      steamPath.value = d.config.steam_path || ''
      proxyUrl.value = d.config.proxy_url || ''
    }
  } catch {
  }
}

async function refreshKernel() {
  try {
    const r = await GetKernelSettings()
    if (r.success) kernel.value = r.settings
  } catch {
  }
}

async function refreshLibrary() {
  try {
    library.value = (await GetLibrary()) || []
  } catch (e: any) {
    addLog('error', e?.message || '读取游戏库失败')
  }
}

async function refreshTask() {
  try {
    const t = await GetTaskStatus()
    taskStatus.value = t.status || 'idle'
    taskResult.value = t.result || null
  } catch {
  }
}

async function refreshKernelStatus() {
  try {
    const r = await KernelStatus()
    kernelState.value = {ready: r.success, message: r.message || (r.success ? '内核已就绪' : '内核缺失')}
  } catch {
    kernelState.value = {ready: false, message: '检测失败'}
  }
}

async function doSearch() {
  const term = searchTerm.value.trim()
  if (!term) {
    addLog('warn', '请输入搜索关键词')
    return
  }
  searching.value = true
  results.value = []
  addLog('info', `正在搜索：${term}`)
  try {
    const res = await SearchStore(term)
    const list = (res && res.items) || []
    results.value = list
    if (list.length) addLog('info', `搜索到 ${list.length} 个结果`)
    else addLog('warn', '未找到匹配的游戏，换个关键词试试')
  } catch (e: any) {
    addLog('error', '搜索失败：' + (e?.message || String(e) || '未知错误'))
    results.value = []
  } finally {
    searching.value = false
  }
}

function formatPrice(r: any) {
  const p = r.price
  if (!p) return '免费'
  return p.final === 0 ? '免费' : '\u00a5' + (p.final / 100).toFixed(2)
}

async function unlock(r: any) {
  try {
    await AddToLibrary(r.id, r.name, r.tiny_image || '', r.type || 'app')
    const resp = await StartUnlock(String(r.id))
    if (resp.success) {
      addLog('info', `已开始解锁：${r.name}`)
    } else {
      addLog('error', resp.message)
    }
  } catch (e: any) {
    addLog('error', e?.message || '解锁失败')
  }
  await refreshTask()
  await refreshLibrary()
}

async function unlockByAppId() {
  const appID = appidTerm.value.trim()
  if (!appID) {
    addLog('warn', '请输入 AppID')
    return
  }
  unlocking.value = true
  try {
    const resp = await StartUnlock(appID)
    if (resp.success) addLog('info', `已开始解锁 AppID：${appID}`)
    else addLog('error', resp.message)
  } catch (e: any) {
    addLog('error', e?.message || '解锁失败')
  } finally {
    unlocking.value = false
  }
  await refreshTask()
  await refreshLibrary()
}

async function addToLibrary(r: any) {
  try {
    const resp = await AddToLibrary(r.id, r.name, r.tiny_image || '', r.type || 'app')
    if (resp.success) addLog('info', `已加入库：${r.name}`)
    else addLog('error', resp.message)
  } catch (e: any) {
    addLog('error', e?.message || '加入库失败')
  }
  await refreshLibrary()
}

async function removeGame(appID: number) {
  try {
    const resp = await RemoveFromLibrary(appID)
    if (resp.success) addLog('info', `已移除 AppID ${appID}`)
    else addLog('error', resp.message)
    if (detailOpen.value === appID) detailOpen.value = null
  } catch (e: any) {
    addLog('error', e?.message || '移除失败')
  }
  await refreshLibrary()
}

async function toggleDetail(appID: number) {
  if (detailOpen.value === appID) {
    detailOpen.value = null
    return
  }
  detailOpen.value = appID
  detail.value = null
  try {
    detail.value = await GetGameDetail(appID)
  } catch (e: any) {
    addLog('error', '加载详情失败：' + (e?.message || String(e)))
  }
}

async function saveConfig() {
  try {
    const cur = (await GetDetailedConfig()).config
    saving.value = true
    const resp = await UpdateConfig({
      steam_path: steamPath.value.trim(),
      debug_mode: cur.debug_mode,
      logging_files: cur.logging_files,
      show_console: cur.show_console,
      language: cur.language,
      proxy_url: proxyUrl.value.trim(),
    })
    if (resp.success) addLog('info', '配置已保存')
    else addLog('error', resp.message)
    await refreshConfig()
  } catch (e: any) {
    addLog('error', '保存失败：' + (e?.message || String(e)))
  } finally {
    saving.value = false
  }
}

async function testProxy() {
  const url = proxyUrl.value.trim()
  if (!url) {
    addLog('warn', '请先填写代理地址')
    return
  }
  testingProxy.value = true
  try {
    const resp = await TestProxy(url)
    if (resp.success) addLog('info', `代理可用：${resp.message}`)
    else addLog('error', `代理不可用：${resp.message}`)
  } catch (e: any) {
    addLog('error', '测试失败：' + (e?.message || String(e)))
  } finally {
    testingProxy.value = false
  }
}

async function saveKernel() {
  try {
    const resp = await SetKernelSettings(kernel.value)
    if (resp.success) addLog('info', '内核设置已保存')
    else addLog('error', resp.message)
  } catch (e: any) {
    addLog('error', '内核设置保存失败：' + (e?.message || String(e)))
  }
}

async function resetConfig() {
  if (!window.confirm('确定要恢复默认配置吗？此操作不可撤销。')) return
  try {
    const resp = await ResetConfig()
    if (resp.success) addLog('info', resp.message)
    else addLog('error', resp.message)
    await refreshConfig()
    await refreshKernel()
  } catch (e: any) {
    addLog('error', '重置失败：' + (e?.message || String(e)))
  }
}

async function installKernel() {
  await act(LoadKernel)
  await refreshKernelStatus()
}

async function restartSteam() {
  await act(RestartSteam)
  await refreshKernelStatus()
}

async function act(fn: () => Promise<any>) {
  try {
    const resp = await fn()
    if (resp.success) addLog('info', resp.message)
    else addLog('error', resp.message)
  } catch (e: any) {
    addLog('error', e?.message || '操作失败')
  }
}

onMounted(async () => {
  await Promise.all([refreshConfig(), refreshKernel(), refreshLibrary(), refreshTask(), refreshKernelStatus()])

  EventsOn('task_progress', (data: any) => {
    if (data && data.message) {
      addLog(data.type || 'info', data.message)
      if (data.type === 'error') taskStatus.value = 'running'
    }
  })
  EventsOn('task_done', async (result: any) => {
    taskStatus.value = result && result.success ? 'completed' : 'error'
    taskResult.value = result || null
    if (result) addLog(result.success ? 'info' : 'error', result.message)
    await refreshLibrary()
  })
})
</script>

<style>
:root {
  --bg: #0d0d17;
  --glass: rgba(255, 255, 255, 0.055);
  --glass-strong: rgba(255, 255, 255, 0.09);
  --stroke: rgba(255, 255, 255, 0.1);
  --text: #eef0ff;
  --text-dim: #9aa0c0;
  --accent: #6c7bff;
  --accent-2: #9d5cff;
  --ok: #41d98a;
  --warn: #f5c14e;
  --err: #ff6b7a;
  --radius: 18px;
  --radius-sm: 12px;
}

* {
  box-sizing: border-box;
}

html, body {
  margin: 0;
  height: 100%;
  background: transparent;
}

body {
  font-family: "Segoe UI", "Microsoft YaHei", system-ui, sans-serif;
  overflow: hidden;
  color: var(--text);
}

/* 星空背景：纯 CSS 多层渐变，无外链资源 */
.app {
  position: relative;
  display: flex;
  flex-direction: column;
  height: 100vh;
  background:
    radial-gradient(1px 1px at 15% 22%, rgba(255,255,255,0.7) 50%, transparent 51%),
    radial-gradient(1px 1px at 62% 9%, rgba(255,255,255,0.55) 50%, transparent 51%),
    radial-gradient(2px 2px at 38% 55%, rgba(255,255,255,0.35) 50%, transparent 51%),
    radial-gradient(1px 1px at 82% 40%, rgba(255,255,255,0.5) 50%, transparent 51%),
    radial-gradient(1px 1px at 28% 78%, rgba(255,255,255,0.4) 50%, transparent 51%),
    radial-gradient(1px 1px at 74% 72%, rgba(255,255,255,0.55) 50%, transparent 51%),
    radial-gradient(2px 2px at 47% 90%, rgba(255,255,255,0.3) 50%, transparent 51%),
    radial-gradient(900px 520px at 50% -10%, rgba(108,123,255,0.22), transparent 60%),
    radial-gradient(720px 480px at 88% 108%, rgba(157,92,255,0.18), transparent 62%),
    linear-gradient(160deg, #0d0d17, #12102b);
  overflow: hidden;
}
.app::before {
  content: "";
  position: absolute;
  inset: 0;
  background: repeating-linear-gradient(115deg, rgba(255,255,255,0.012) 0 2px, transparent 2px 6px);
  pointer-events: none;
}

.titlebar {
  position: relative;
  z-index: 2;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 8px 0 16px;
  background: linear-gradient(180deg, rgba(255,255,255,0.06), rgba(255,255,255,0.015));
  border-bottom: 1px solid var(--stroke);
  user-select: none;
  flex-shrink: 0;
}
.titlebar-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 700;
  letter-spacing: 0.4px;
  background: linear-gradient(90deg, var(--accent), var(--accent-2));
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}
.dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
  background: linear-gradient(135deg, var(--accent), var(--accent-2));
  box-shadow: 0 0 10px rgba(108,123,255,0.8);
  -webkit-background-clip: padding-box;
  background-clip: padding-box;
}
.titlebar-controls {
  display: flex;
  height: 100%;
}
.tb-btn {
  width: 46px;
  border: none;
  background: transparent;
  color: var(--text-dim);
  font-size: 14px;
  cursor: pointer;
}
.tb-btn:hover {
  background: rgba(255, 255, 255, 0.1);
  color: var(--text);
}
.tb-close:hover {
  background: #e81123;
  color: #fff;
}

.statusbar {
  position: relative;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 18px;
  padding: 10px 20px;
  font-size: 12.5px;
  color: var(--text-dim);
}
.status-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.status-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #4a4f6e;
}
.status-dot.ok { background: var(--ok); box-shadow: 0 0 8px var(--ok); }
.status-dot.running { background: var(--warn); box-shadow: 0 0 8px var(--warn); animation: pulse 1.2s infinite; }
.status-dot.error { background: var(--err); box-shadow: 0 0 8px var(--err); }
.status-item.ok { color: var(--ok); }
.status-item.running { color: var(--warn); }
.status-item.error { color: var(--err); }
.status-item.task-msg { color: var(--text-dim); max-width: 46vw; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1; }
@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.3; }
}

.content {
  position: relative;
  z-index: 2;
  flex: 1;
  overflow-y: auto;
  padding: 4px 20px 26px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.card {
  background: var(--glass);
  border: 1px solid var(--stroke);
  border-radius: var(--radius);
  padding: 18px 20px;
  backdrop-filter: blur(14px) saturate(140%);
  -webkit-backdrop-filter: blur(14px) saturate(140%);
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.25);
}
.card h2 {
  margin: 0 0 14px;
  font-size: 14px;
  font-weight: 700;
  color: var(--text);
  letter-spacing: 0.3px;
}

.grid-2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 12.5px;
  color: var(--text-dim);
}
.row {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  margin-top: 14px;
}
.row input {
  flex: 1;
  min-width: 220px;
  margin-top: 0;
}
input {
  background: rgba(0, 0, 0, 0.28);
  border: 1px solid var(--stroke);
  color: var(--text);
  border-radius: var(--radius-sm);
  padding: 9px 13px;
  font-size: 13.5px;
  outline: none;
  transition: border-color 0.15s;
}
input::placeholder { color: #6a6f8c; }
input:focus { border-color: var(--accent); }

button {
  background: var(--glass-strong);
  border: 1px solid var(--stroke);
  color: var(--text);
  border-radius: var(--radius-sm);
  padding: 9px 16px;
  font-size: 13.5px;
  cursor: pointer;
  transition: transform 0.05s ease, background 0.15s, border-color 0.15s;
}
button:hover {
  background: rgba(255, 255, 255, 0.14);
  border-color: rgba(255, 255, 255, 0.2);
}
button:active { transform: translateY(1px); }
button.primary {
  background: linear-gradient(135deg, var(--accent), var(--accent-2));
  border: none;
  font-weight: 600;
}
button.primary:hover { filter: brightness(1.1); }
button.ghost { opacity: 0.8; }
button.danger {
  background: rgba(255, 107, 122, 0.14);
  border-color: rgba(255, 107, 122, 0.35);
  color: #ffb4bd;
}
button.danger:hover { background: rgba(255, 107, 122, 0.25); }
button:disabled { opacity: 0.55; cursor: not-allowed; }

.switches {
  display: flex;
  flex-wrap: wrap;
  gap: 18px;
}
.switch {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  cursor: pointer;
  color: var(--text);
}
.switch input {
  position: absolute;
  opacity: 0;
  width: 0;
  height: 0;
}
.switch .track {
  width: 40px;
  height: 22px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.14);
  position: relative;
  transition: background 0.2s;
  flex-shrink: 0;
}
.switch .thumb {
  position: absolute;
  top: 3px;
  left: 3px;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: #fff;
  transition: left 0.2s;
}
.switch input:checked + .track {
  background: linear-gradient(135deg, var(--accent), var(--accent-2));
}
.switch input:checked + .track .thumb { left: 21px; }

.results {
  margin-top: 14px;
  display: flex;
  flex-direction: column;
  gap: 9px;
}
.item, .lib-item {
  display: flex;
  align-items: center;
  gap: 12px;
  background: rgba(0, 0, 0, 0.24);
  border: 1px solid var(--stroke);
  border-radius: var(--radius-sm);
  padding: 8px;
}
.item img, .lib-item img {
  width: 60px;
  height: 34px;
  object-fit: cover;
  border-radius: 7px;
  background: rgba(0,0,0,0.4);
  flex-shrink: 0;
}
.item .info, .lib-item .info {
  flex: 1;
  overflow: hidden;
}
.name {
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  display: flex;
  align-items: center;
  gap: 8px;
}
.meta {
  font-size: 12px;
  color: var(--text-dim);
}
.platforms { color: var(--text-dim); }
.badge {
  font-style: normal;
  font-size: 10.5px;
  padding: 2px 7px;
  border-radius: 999px;
  background: rgba(65, 217, 138, 0.16);
  color: var(--ok);
  border: 1px solid rgba(65, 217, 138, 0.4);
  flex-shrink: 0;
}
.item .actions, .lib-item .actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.lib-item {
  flex-wrap: wrap;
}
.detail {
  width: 100%;
  margin-top: 6px;
  padding-top: 10px;
  border-top: 1px dashed var(--stroke);
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.detail-block { display: flex; flex-direction: column; gap: 4px; }
.detail-h { font-size: 12px; color: var(--text-dim); font-weight: 600; }
.detail-row {
  display: flex;
  gap: 16px;
  font-size: 12px;
  color: var(--text);
}
.mono { font-family: Consolas, "Cascadia Mono", monospace; }
.dim { color: var(--text-dim); }
.detail-note { font-size: 12px; color: var(--text-dim); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.empty {
  color: var(--text-dim);
  font-size: 13px;
}
.hint {
  margin-top: 12px;
  font-size: 12.5px;
  color: var(--text-dim);
}
.kernel-state {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  margin-top: 12px;
  font-size: 13px;
}
.kernel-state.ok { color: var(--ok); }
.kernel-state.err { color: var(--err); }

.av-guide {
  margin-top: 10px;
  font-size: 12.5px;
  border: 1px solid var(--stroke);
  border-radius: 8px;
  padding: 6px 10px;
  color: var(--text-dim);
}
.av-guide summary {
  cursor: pointer;
  color: var(--text);
  font-weight: 500;
}
.av-guide-body { margin-top: 8px; }
.av-guide-body p { margin: 6px 0; line-height: 1.5; }
.av-guide-body code { background: var(--stroke); border-radius: 4px; padding: 1px 4px; }
.av-tip { font-size: 12px; }

.log-line {
  font-size: 12.5px;
  padding: 4px 0;
  border-bottom: 1px solid var(--stroke);
  font-family: Consolas, "Cascadia Mono", monospace;
}
.log-line:last-child { border-bottom: none; }
.log-line.info { color: var(--ok); }
.log-line.error { color: var(--err); }
.log-line.warn { color: var(--warn); }
.log-line .time {
  color: var(--text-dim);
  margin-right: 8px;
}

@media (max-width: 720px) {
  .grid-2 { grid-template-columns: 1fr; }
}
</style>