# Third-Party Notices

MyWAN는 Apache-2.0로 배포되며, 아래 제3자 구성요소를 사용한다. 모든 의존성은 **퍼미시브
라이선스(MIT/BSD/Apache/ISC/Public-Domain)** 만 채택한다. **GPL/AGPL/LGPL 등 카피레프트
라이선스 의존은 금지**한다(소스 공개 강제 회피). 이 목록은 앱 About 화면에도 표기한다.

> 이 파일은 실제 의존성 추가에 맞춰 갱신한다. 아래는 계획된 핵심 구성요소.

| 구성요소 | 용도 | 라이선스 | 비고 |
|----------|------|----------|------|
| golang.zx2c4.com/wireguard (wireguard-go) | 암호화 터널 데이터 평면 | MIT | |
| golang.zx2c4.com/wintun (Go 바인딩) | Windows 가상 NIC | MIT (바인딩) | |
| Wintun (prebuilt `wintun.dll`) | Windows TUN 드라이버 | 재배포 허용(프리빌트) | 드라이버 '소스'는 GPLv2. **서명된 프리빌트 dll을 C API로 동적 로드**(Tailscale 방식) → 앱은 GPL 파생물 아님. 재배포 약관 최종 확인 필요 |
| modernc.org/sqlite | 서버 내장 저장소 | BSD-3 | SQLite 자체 Public-Domain, 순수 Go(cgo 불필요) |
| github.com/miekg/dns | 내부 DNS | BSD-3 | |
| fyne.io/systray | 트레이 상주 | Apache-2.0 / MIT | |
| github.com/wailsapp/wails/v2 | 데스크톱 GUI 셸 | MIT | WebView2 런타임 = Microsoft 재배포 허용(프로프라이어터리 런타임) |
| React, Vite, Tailwind CSS, Radix UI, shadcn/ui | GUI 프론트엔드 | MIT | shadcn/ui는 복사형 컴포넌트(MIT) |
| golang.org/x/sys, golang.org/x/crypto | 시스템/암호 유틸, 컨트롤 API TLS(`acme/autocert`) | BSD-3 | ACME 클라이언트가 표준 라이브러리 계열이라 별도 의존성 추가 없음 |

## 금지 목록 (예시)
- `wireguard-windows` 앱 코드(GPLv2) — **차용 금지**(참고만, 재구현).
- 임의의 GPL/AGPL/LGPL 라이브러리.

## 검증 방법
- 의존성 추가 시 `go-licenses`(또는 동등 도구)로 라이선스 스캔을 CI에 포함.
- 프론트엔드는 `license-checker`로 npm 의존성 스캔.
