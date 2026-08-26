/**
 * Translations for the desktop app.
 *
 * English is the source of truth: every other language is typed as a complete
 * record of its keys, so adding an English string and forgetting to translate it
 * is a compile error rather than a word that quietly stays English on a Korean
 * machine.
 *
 * What is deliberately NOT translated: addresses, tokens, pins, CIDRs, command
 * lines and the names an operator chose. They are the same in every language,
 * and a translated command is a command that does not work. Messages coming back
 * from the engine and the control server stay in their original wording too —
 * they are diagnostics, and matching one against a search or a log is worth more
 * than reading it in your own language.
 */

export type Lang = 'en' | 'ko'

export const languages: { code: Lang; label: string }[] = [
  { code: 'en', label: 'English' },
  { code: 'ko', label: '한국어' },
]

const en = {
  // --- shell -----------------------------------------------------------------
  'app.subtitle': 'private overlay network',
  'app.tab.join': 'Join',
  'app.tab.host': 'Host',
  'app.update.available': 'Update available:',
  'app.update.now': 'Update now',
  'app.update.running': 'Updating…',
  'app.theme.title': 'Theme: {theme} (click to change)',
  'app.theme.system': 'system',
  'app.theme.light': 'light',
  'app.theme.dark': 'dark',
  'app.language.title': 'Language: {language} (click to change)',

  // --- a view that threw -----------------------------------------------------
  'error.title': 'This view failed to render',
  'error.body':
    'The rest of the app still works — switch tabs and come back, or reopen the window. Nothing about your networks has changed.',
  'error.retry': 'Try again',

  // --- shared ----------------------------------------------------------------
  'common.copy': 'Copy',
  'common.generate': 'Generate',
  'common.remove': 'Remove',
  'common.cancel': 'Cancel',
  'common.unnamed': '(unnamed)',
  'common.relay': 'Relay',

  // --- join ------------------------------------------------------------------
  'client.serviceDown':
    'Engine service is not running. Install it (the installer does this), or run {command} as Administrator.',
  'client.add.title': 'Join a network',
  'client.add.description': 'Paste the join address the host gave you, with your token.',
  'client.alias': 'Local name',
  'client.alias.help':
    'Your own short name for this network. Services live under it — {example} — so it keeps two networks apart even when both servers use the same suffix.',
  'client.server': 'Server',
  'client.server.insecure': 'Plain http: the token and control traffic are sent unencrypted.',
  'client.pin': 'Server key pin',
  'client.pin.placeholder': 'sha256:… (leave empty for a public domain)',
  'client.token': 'Token',
  'client.token.placeholder': 'join token',
  'client.deviceName': "This device's name (optional)",
  'client.useRelay': 'Route via server relay (works behind NAT)',
  'client.join': 'Join',
  'client.joining': 'Joining…',
  'client.networks': 'Networks',
  'client.connectedCount': '{connected} of {total} connected',
  'client.joinAnother': 'Join another',
  'client.state.connected': 'connected',
  'client.state.disconnected': 'disconnected',
  'client.via': 'via {via}',
  'client.leave': 'Leave',
  'client.forget': 'Forget this network',
  'client.trust.plaintext': 'plaintext — not authenticated',
  'client.trust.pinned': 'TLS · pinned key',
  'client.trust.ca': 'TLS · CA certificate',
  'client.row.controlServer': 'Control server',
  'client.row.names': 'Names',
  'client.row.addressHere': 'Address on this device',
  'client.row.addressInNetwork': 'Address in the network',
  'client.row.localRange': 'Local range',
  'client.row.overlayCidr': 'Overlay CIDR',
  'client.peers': 'Peers',
  'client.peers.none': 'No peers yet.',
  'client.peer.established': 'tunnel established',
  'client.peer.connecting': 'connecting',
  'client.services': 'Services',
  'client.services.none': 'No services published.',
  'client.publish': 'Published by this device',
  'client.publish.help':
    'Offer something on this machine to the network. Members reach it at {example} on the port you give it, and the program behind it stays on localhost.',
  'client.publish.none': 'Nothing published from this device.',
  'client.publish.add': 'Publish a service',
  'client.publish.name': 'Name',
  'client.publish.namePlaceholder': 'minecraft',
  'client.publish.port': 'Port',
  'client.publish.portHelp': 'the port members connect to',
  'client.publish.backend': 'Local port',
  'client.publish.backendHelp': 'the port it already listens on here; empty means it binds the overlay itself',
  'client.publish.proto': 'Protocol',
  'client.publish.groups': 'Allowed groups',
  'client.publish.groupsPlaceholder': 'everyone who can reach this device',
  'client.publish.save': 'Save and reconnect',
  'client.publish.saving': 'Reconnecting…',
  'client.publish.pending': 'Saving reconnects this network so the changes take effect.',
  'client.isolationNote':
    'Each network gets its own adapter, key, address range and names, and nothing routes between them — so two networks may use the same overlay range without colliding here.',

  // --- host ------------------------------------------------------------------
  'host.serviceNotice':
    'The control-server service is not installed, so the network runs inside this app and stops when you quit. Re-run the installer and tick {server}, or run {command} as Administrator.',
  'host.serviceNotice.server': 'Server',
  'host.title': 'Host a network',
  'host.description': 'Run a control server + relay on this machine (needs a public IP for remote clients).',
  'host.network': 'Network',
  'host.dnsSuffix': 'DNS suffix',
  'host.cidr': 'Overlay CIDR',
  'host.joinToken': 'Join token',
  'host.controlAddr': 'Control addr',
  'host.relayAddr': 'Relay addr',
  'host.tls': 'TLS',
  'host.tls.auto': 'Automatic — certificate for the domain, otherwise a pinned key',
  'host.tls.self': 'Self-signed key, clients pin it',
  'host.tls.acme': 'Public certificate (ACME) for the domain',
  'host.tls.off': 'Off — plaintext HTTP (local testing only)',
  'host.tls.offWarning': 'Join tokens and all control traffic would be sent unencrypted.',
  'host.domain': 'Domain (optional)',
  'host.domain.placeholder': 'vpn.example.com — leave empty to use a pinned key',
  'host.domain.help':
    'With a domain pointed at this machine, a certificate is issued automatically (needs port 443, or port 80 reachable).',
  'host.publicHost': 'Public address (optional)',
  'host.publicHost.placeholder': '203.0.113.5:8787 — what clients should connect to',
  'host.acl': 'Access control',
  'host.acl.help': 'Optional. Each group gets its own join token; the token above puts members in {default}.',
  'host.acl.groupPlaceholder': 'group',
  'host.acl.addGroup': 'Add group',
  'host.acl.rules': 'Rules',
  'host.acl.rulesHelp':
    'One {form} per line. No rules means everyone reaches everyone; the first rule makes everything else denied.',
  'host.start': 'Start hosting',
  'host.badge.hosting': 'hosting',
  'host.inBackground': 'runs in the background service',
  'host.stop': 'Stop',
  'host.share': 'Share with clients',
  'host.share.joinAddress': 'Join address',
  'host.share.token': 'Token',
  'host.share.control': 'Control',
  'host.share.trust': 'Trust',
  'host.share.copyPin': 'Copy pin',
  'host.trust.off': 'plaintext — no TLS',
  'host.trust.acme': 'ACME certificate',
  'host.share.pinNote': 'The join address already contains the key pin — clients can paste it into the Server field as-is.',
  'host.share.loopbackNote':
    'It points at this machine. Set a public address and start again so remote clients get a reachable host.',
  'host.acl.noRules': 'No rules — every member reaches every other.',
  'host.acl.denyExcept': 'Default-deny except:',
  'host.acl.groups': 'Groups',
  'host.acl.groupToken': 'Token · {group}',
  'host.members': 'Members',
  'host.members.none': 'No members yet.',
  'host.services': 'Services',
  'host.services.none': 'No services published.',
} as const

export type Key = keyof typeof en

const ko: Record<Key, string> = {
  // --- shell -----------------------------------------------------------------
  'app.subtitle': '프라이빗 오버레이 네트워크',
  'app.tab.join': '가입',
  'app.tab.host': '호스팅',
  'app.update.available': '업데이트 있음:',
  'app.update.now': '지금 업데이트',
  'app.update.running': '업데이트 중…',
  'app.theme.title': '테마: {theme} (클릭해서 변경)',
  'app.theme.system': '시스템',
  'app.theme.light': '밝게',
  'app.theme.dark': '어둡게',
  'app.language.title': '언어: {language} (클릭해서 변경)',

  // --- a view that threw -----------------------------------------------------
  'error.title': '이 화면을 그리지 못했습니다',
  'error.body':
    '나머지 기능은 그대로 동작합니다. 탭을 옮겼다 돌아오거나 창을 다시 열어 보세요. 네트워크 설정은 아무것도 바뀌지 않았습니다.',
  'error.retry': '다시 시도',

  // --- shared ----------------------------------------------------------------
  'common.copy': '복사',
  'common.generate': '새로 생성',
  'common.remove': '삭제',
  'common.cancel': '취소',
  'common.unnamed': '(이름 없음)',
  'common.relay': '릴레이',

  // --- join ------------------------------------------------------------------
  'client.serviceDown':
    '엔진 서비스가 실행 중이 아닙니다. 설치 프로그램으로 설치하거나, 관리자 권한에서 {command} 을(를) 실행하세요.',
  'client.add.title': '네트워크 가입',
  'client.add.description': '호스트가 알려준 가입 주소를 토큰과 함께 붙여넣으세요.',
  'client.alias': '로컬 이름',
  'client.alias.help':
    '이 네트워크를 부를 나만의 짧은 이름입니다. 서비스가 이 이름 아래에 놓이므로({example}), 두 서버가 같은 서픽스를 쓰더라도 네트워크가 서로 구분됩니다.',
  'client.server': '서버',
  'client.server.insecure': '평문 http 입니다. 토큰과 컨트롤 트래픽이 암호화되지 않은 채 전송됩니다.',
  'client.pin': '서버 키 지문',
  'client.pin.placeholder': 'sha256:… (공인 도메인이면 비워 두세요)',
  'client.token': '토큰',
  'client.token.placeholder': '가입 토큰',
  'client.deviceName': '이 기기의 이름 (선택)',
  'client.useRelay': '서버 릴레이 경유 (NAT 뒤에서도 동작)',
  'client.join': '가입',
  'client.joining': '가입 중…',
  'client.networks': '네트워크',
  'client.connectedCount': '{total}개 중 {connected}개 연결됨',
  'client.joinAnother': '다른 네트워크 가입',
  'client.state.connected': '연결됨',
  'client.state.disconnected': '연결 안 됨',
  'client.via': '{via} 경유',
  'client.leave': '나가기',
  'client.forget': '이 네트워크 삭제',
  'client.trust.plaintext': '평문 — 서버 신원 미확인',
  'client.trust.pinned': 'TLS · 키 지문 고정',
  'client.trust.ca': 'TLS · CA 인증서',
  'client.row.controlServer': '컨트롤 서버',
  'client.row.names': '이름',
  'client.row.addressHere': '이 기기에서의 주소',
  'client.row.addressInNetwork': '네트워크 안에서의 주소',
  'client.row.localRange': '로컬 대역',
  'client.row.overlayCidr': '오버레이 CIDR',
  'client.peers': '피어',
  'client.peers.none': '아직 피어가 없습니다.',
  'client.peer.established': '터널 연결됨',
  'client.peer.connecting': '연결 중',
  'client.services': '서비스',
  'client.services.none': '게시된 서비스가 없습니다.',
  'client.publish': '이 기기에서 게시',
  'client.publish.help':
    '이 기계의 무언가를 네트워크에 내놓습니다. 멤버는 지정한 포트로 {example} 에 접속하고, 뒤에 있는 프로그램은 localhost 에 그대로 둡니다.',
  'client.publish.none': '이 기기에서 게시한 것이 없습니다.',
  'client.publish.add': '서비스 게시',
  'client.publish.name': '이름',
  'client.publish.namePlaceholder': 'minecraft',
  'client.publish.port': '포트',
  'client.publish.portHelp': '멤버가 접속할 포트',
  'client.publish.backend': '로컬 포트',
  'client.publish.backendHelp': '이 기계에서 이미 열려 있는 포트. 비우면 프로그램이 오버레이 주소에 직접 바인딩합니다',
  'client.publish.proto': '프로토콜',
  'client.publish.groups': '허용 그룹',
  'client.publish.groupsPlaceholder': '이 기기에 닿는 모든 멤버',
  'client.publish.save': '저장하고 재접속',
  'client.publish.saving': '재접속 중…',
  'client.publish.pending': '저장하면 이 네트워크를 재접속해 변경이 적용됩니다.',
  'client.isolationNote':
    '네트워크마다 어댑터·키·주소 대역·이름이 따로 주어지고 서로 라우팅되지 않습니다. 그래서 두 네트워크가 같은 오버레이 대역을 써도 이 기기에서는 충돌하지 않습니다.',

  // --- host ------------------------------------------------------------------
  'host.serviceNotice':
    '컨트롤 서버 서비스가 설치돼 있지 않습니다. 네트워크가 이 앱 안에서 돌기 때문에 앱을 종료하면 함께 멈춥니다. 설치 프로그램을 다시 실행해 {server} 를 선택하거나, 관리자 권한에서 {command} 을(를) 실행하세요.',
  'host.serviceNotice.server': 'Server',
  'host.title': '네트워크 호스팅',
  'host.description': '이 기기에서 컨트롤 서버와 릴레이를 운영합니다 (외부 클라이언트를 받으려면 공인 IP 가 필요합니다).',
  'host.network': '네트워크',
  'host.dnsSuffix': 'DNS 서픽스',
  'host.cidr': '오버레이 CIDR',
  'host.joinToken': '가입 토큰',
  'host.controlAddr': '컨트롤 주소',
  'host.relayAddr': '릴레이 주소',
  'host.tls': 'TLS',
  'host.tls.auto': '자동 — 도메인이 있으면 인증서, 없으면 키 지문 고정',
  'host.tls.self': '자체 서명 키, 클라이언트가 지문으로 검증',
  'host.tls.acme': '도메인에 대한 공인 인증서 (ACME)',
  'host.tls.off': '끄기 — 평문 HTTP (로컬 테스트 전용)',
  'host.tls.offWarning': '가입 토큰과 모든 컨트롤 트래픽이 암호화되지 않은 채 전송됩니다.',
  'host.domain': '도메인 (선택)',
  'host.domain.placeholder': 'vpn.example.com — 비워 두면 키 지문 고정 방식',
  'host.domain.help': '이 기기를 가리키는 도메인이 있으면 인증서가 자동 발급됩니다 (443 포트, 또는 80 포트가 열려 있어야 합니다).',
  'host.publicHost': '공개 주소 (선택)',
  'host.publicHost.placeholder': '203.0.113.5:8787 — 클라이언트가 접속할 주소',
  'host.acl': '접근 제어',
  'host.acl.help': '선택 사항입니다. 그룹마다 별도의 가입 토큰을 갖습니다. 위의 토큰으로 들어온 멤버는 {default} 그룹이 됩니다.',
  'host.acl.groupPlaceholder': '그룹',
  'host.acl.addGroup': '그룹 추가',
  'host.acl.rules': '규칙',
  'host.acl.rulesHelp':
    '한 줄에 {form} 하나씩. 규칙이 하나도 없으면 모두가 서로에게 닿고, 규칙이 하나라도 생기면 나머지는 모두 차단됩니다.',
  'host.start': '호스팅 시작',
  'host.badge.hosting': '호스팅 중',
  'host.inBackground': '백그라운드 서비스에서 실행 중',
  'host.stop': '중지',
  'host.share': '클라이언트에게 공유',
  'host.share.joinAddress': '가입 주소',
  'host.share.token': '토큰',
  'host.share.control': '컨트롤',
  'host.share.trust': '신뢰',
  'host.share.copyPin': '지문 복사',
  'host.trust.off': '평문 — TLS 없음',
  'host.trust.acme': 'ACME 인증서',
  'host.share.pinNote': '가입 주소에 키 지문이 이미 들어 있습니다. 클라이언트는 그대로 서버 칸에 붙여넣으면 됩니다.',
  'host.share.loopbackNote': '이 주소는 이 기기를 가리킵니다. 공개 주소를 지정하고 다시 시작해야 외부 클라이언트가 접속할 수 있습니다.',
  'host.acl.noRules': '규칙 없음 — 모든 멤버가 서로에게 닿습니다.',
  'host.acl.denyExcept': '기본 차단, 다음만 허용:',
  'host.acl.groups': '그룹',
  'host.acl.groupToken': '토큰 · {group}',
  'host.members': '멤버',
  'host.members.none': '아직 멤버가 없습니다.',
  'host.services': '서비스',
  'host.services.none': '게시된 서비스가 없습니다.',
}

const dictionaries: Record<Lang, Record<Key, string>> = { en, ko }

const KEY = 'zwan-lang'

/** detect picks a language from the system when the user has not chosen one. */
function detect(): Lang {
  try {
    for (const tag of navigator.languages ?? [navigator.language]) {
      if (tag?.toLowerCase().startsWith('ko')) return 'ko'
    }
  } catch {
    /* no navigator in some hosts */
  }
  return 'en'
}

export function getStoredLang(): Lang {
  try {
    const v = localStorage.getItem(KEY)
    if (v === 'en' || v === 'ko') return v
  } catch {
    /* storage may be unavailable */
  }
  return detect()
}

let current: Lang = 'en'
const listeners = new Set<() => void>()

/** subscribe/snapshot back useSyncExternalStore, so a change repaints everything. */
export function subscribe(fn: () => void): () => void {
  listeners.add(fn)
  return () => listeners.delete(fn)
}

export function snapshot(): Lang {
  return current
}

export function setLang(lang: Lang) {
  current = lang
  try {
    localStorage.setItem(KEY, lang)
  } catch {
    /* ignore */
  }
  try {
    document.documentElement.lang = lang
  } catch {
    /* ignore */
  }
  for (const fn of listeners) fn()
}

export function initLang() {
  setLang(getStoredLang())
}

/**
 * translate looks up a key and fills in {placeholders}.
 *
 * A missing translation falls back to English rather than showing the key: a
 * word in the wrong language is readable, and a key is not.
 */
export function translate(lang: Lang, key: Key, vars?: Record<string, string | number>): string {
  const text = dictionaries[lang]?.[key] ?? en[key] ?? key
  if (!vars) return text
  return text.replace(/\{(\w+)\}/g, (whole, name) => (name in vars ? String(vars[name]) : whole))
}
