'use strict';

/* ============================================================
   W1nWire UI — vanilla JS, no build step, no dependencies.
   Works against the local Go HTTP API, or entirely from memory
   when opened as index.html?mock=1 (or #mock).
   ============================================================ */

const I18N = {
  ru: {
    'nav.aria': 'Основная навигация',
    'nav.connect': 'Подключение',
    'nav.servers': 'Серверы',
    'nav.settings': 'Настройки',
    'nav.logs': 'Журнал',
    'nav.about': 'О программе',
    'sidebar.protocol': 'Собственный протокол',
    'sidebar.version': 'Версия',
    'top.noServer': 'Сервер не выбран',
    'top.noAddress': 'Адрес не указан',
    'top.notElevated': 'Без прав администратора',
    'top.openServers': 'Открыть список серверов',
    'top.language': 'Язык интерфейса',
    'state.disconnected': 'Отключено',
    'state.connecting': 'Подключение…',
    'state.connected': 'Защищено',
    'state.reconnecting': 'Переподключение…',
    'state.disconnecting': 'Отключение…',
    'state.error': 'Ошибка',
    'state.eyebrow': 'Состояние',
    'power.connect': 'Подключить',
    'power.disconnect': 'Отключить',
    'power.cancel': 'Отменить',
    'power.disconnecting': 'Отключение…',
    'power.pending': 'Подождите…',
    'caption.disconnected': 'Готово к подключению',
    'caption.connecting': 'Устанавливаем защищённый канал',
    'caption.connected': 'Туннель активен',
    'caption.proxy': 'SOCKS5-прокси активен',
    'caption.disconnecting': 'Закрываем защищённый канал',
    'caption.error': 'Проверьте журнал и повторите',
    'conn.ip': 'Назначенный IP',
    'conn.uptime': 'Время сессии',
    'conn.fallbackServer': 'Сервер не выбран',
    'conn.fallbackAddress': 'Адрес не указан',
    'conn.error': 'Не удалось подключиться. Попробуйте ещё раз.',
    'mode.group': 'Режим подключения',
    'mode.tunnel': 'Туннель',
    'mode.proxy': 'Прокси',
    'mode.tunnelDesc': 'Весь трафик устройства проходит через зашифрованный туннель W1nWire.',
    'mode.proxyDesc': 'Приложения используют локальный SOCKS5-прокси, остальной трафик идёт напрямую.',
    'mode.locked': 'Отключитесь, чтобы сменить режим.',
    'mode.proxyAddress': 'Адрес прокси',
    'mode.systemProxy': 'Системный прокси',
    'stats.downloaded': 'Скачано',
    'stats.uploaded': 'Отправлено',
    'stats.speed': 'Скорость',
    'stats.latency': 'Задержка',
    'stats.session': 'за сессию',
    'stats.toServer': 'до сервера',
    'stats.last60': 'Последние 60 секунд',
    'stats.rx': 'Приём',
    'stats.tx': 'Отдача',
    'stats.na': '—',
    'stats.ms': 'мс',
    'notices.adminTitle': 'Для режима туннеля нужны права администратора',
    'notices.adminText': 'Перезапустите W1nWire от имени администратора, чтобы включить туннель.',
    'notices.relaunch': 'Перезапустить с правами администратора',
    'notices.wintunMissing': 'Драйвер Wintun не найден',
    'notices.wintunMissingText': 'Без него туннельный режим недоступен. Файл будет загружен в папку приложения.',
    'notices.wintunError': 'Ошибка драйвера Wintun',
    'notices.wintunErrorText': 'Проверьте подключение к интернету и попробуйте снова.',
    'notices.wintunDownloading': 'Загрузка wintun.dll…',
    'notices.download': 'Скачать драйвер (wintun.dll)',
    'notices.retry': 'Повторить',
    'notices.wintunVersion': 'Версия {version}',
    'empty.eyebrow': 'Первый запуск',
    'empty.title': 'Добавьте сервер',
    'empty.desc': 'Вставьте код-приглашение или добавьте сервер вручную в разделе «Серверы». Ключи создаются локально и не покидают устройство.',
    'empty.invite': 'Вставить код-приглашение',
    'empty.free': 'Как получить бесплатный сервер',
    'empty.foot': 'Код-приглашение начинается с ww1. или w1nwire://',
    'servers.title': 'Серверы',
    'servers.desc': 'Профили подключения. Нажмите на сервер, чтобы выбрать его.',
    'servers.add': 'Добавить сервер',
    'servers.import': 'Импорт кода',
    'servers.active': 'Активный',
    'servers.connected': 'Подключён',
    'servers.connect': 'Подключить',
    'servers.disconnect': 'Отключить',
    'servers.test': 'Проверить',
    'servers.edit': 'Изменить',
    'servers.delete': 'Удалить',
    'servers.unnamed': 'Сервер',
    'servers.activateAria': 'Сделать сервер «{name}» активным',
    'servers.connectAria': 'Подключиться к серверу «{name}»',
    'servers.testAria': 'Проверить задержку сервера «{name}»',
    'servers.editAria': 'Изменить сервер «{name}»',
    'servers.deleteAria': 'Удалить сервер «{name}»',
    'servers.emptyTitle': 'Пока нет серверов',
    'servers.emptyDesc': 'Добавьте профиль вручную или вставьте код-приглашение, чтобы начать.',
    'servers.testing': 'Проверка…',
    'servers.testFail': 'Не удалось проверить сервер',
    'settings.title': 'Настройки',
    'settings.desc': 'Сеть, запуск и локальные данные приложения.',
    'settings.group.general': 'Общие',
    'settings.group.network': 'Сеть',
    'settings.group.startup': 'Запуск и поведение',
    'settings.group.data': 'Данные и диагностика',
    'settings.language': 'Язык интерфейса',
    'settings.languageDesc': 'Применяется сразу, без перезапуска.',
    'settings.dns': 'DNS-сервер',
    'settings.dnsDesc': 'Используется внутри туннеля. Один IPv4-адрес.',
    'settings.dnsPlaceholder': '1.1.1.1',
    'settings.mtu': 'MTU',
    'settings.mtuDesc': 'Размер пакета туннеля. Рекомендуется 1420.',
    'settings.proxyPort': 'Порт прокси',
    'settings.proxyPortDesc': 'Локальный SOCKS5-порт для режима «Прокси».',
    'settings.blockIPv6': 'Блокировать IPv6',
    'settings.blockIPv6Desc': 'Предотвращает утечку IPv6-трафика вне туннеля.',
    'settings.autoStart': 'Автозапуск при входе',
    'settings.autoStartDesc': 'Запускать W1nWire при входе в Windows.',
    'settings.autoConnect': 'Подключаться при запуске',
    'settings.autoConnectDesc': 'Автоматически подключать активный профиль.',
    'settings.minimizeToTray': 'Сворачивать в трей',
    'settings.minimizeToTrayDesc': 'Недоступно в этой версии.',
    'settings.dataDir': 'Папка данных',
    'settings.dataDirDesc': 'Профили, журналы и локальные ключи.',
    'settings.copyPath': 'Копировать путь',
    'settings.openData': 'Открыть папку данных',
    'settings.openLogs': 'Открыть папку логов',
    'settings.reset': 'Сбросить настройки',
    'settings.selftest': 'Проверить установку',
    'settings.selftestRunning': 'Проверка…',
    'settings.selftestDesc': 'Проверит драйвер, права, порт прокси и файлы конфигурации.',
    'settings.invalidValue': 'Проверьте значение поля.',
    'settings.resetConfirmTitle': 'Сбросить настройки?',
    'settings.resetConfirmText': 'Все настройки вернутся к значениям по умолчанию. Профили и журналы останутся на месте.',
    'settings.resetDone': 'Настройки сброшены',
    'logs.title': 'Журнал',
    'logs.desc': 'События подключения, драйвера и приложения. Данные хранятся локально.',
    'logs.all': 'Все',
    'logs.level.info': 'Инфо',
    'logs.level.warn': 'Внимание',
    'logs.level.error': 'Ошибка',
    'logs.autoScroll': 'Автоскролл',
    'logs.copy': 'Копировать',
    'logs.clear': 'Очистить',
    'logs.empty': 'Событий пока нет',
    'logs.emptyFiltered': 'Нет событий с этим уровнем',
    'logs.copied': 'Журнал скопирован',
    'logs.nothingToCopy': 'В журнале нет событий для копирования.',
    'logs.clearTitle': 'Очистить журнал?',
    'logs.clearText': 'Все записи будут удалены без возможности восстановления.',
    'logs.copyAria': 'Копировать журнал',
    'logs.clearAria': 'Очистить журнал',
    'logs.cleared': 'Журнал очищен',
    'about.tagline': 'Личный VPN-клиент для Windows. Один исполняемый файл, собственный протокол и никакой лишней телеметрии.',
    'about.version': 'Версия {version}',
    'about.protocolLabel': 'Протокол',
    'about.protocol': 'собственный, X25519 + ChaCha20-Poly1305',
    'about.resources': 'Ресурсы',
    'about.github': 'GitHub',
    'about.docs': 'Документация',
    'about.license': 'Лицензия MIT',
    'about.openExternal': 'Открыть {name}',
    'about.privacy': 'Конфиденциальность',
    'about.privacy.noTelemetry': 'Нет телеметрии: приложение никуда не отправляет данные о вас.',
    'about.privacy.noAccounts': 'Нет аккаунтов и подписок: всё работает на вашем устройстве.',
    'about.privacy.localKeys': 'Приватные ключи не покидают устройство и лежат только в вашем профиле.',
    'about.privacy.localLogs': 'Журнал событий хранится локально и очищается одной кнопкой.',
    'about.privacy.openSource': 'Код открыт и под лицензией MIT: его можно прочитать целиком.',
    'about.disclaimerTitle': 'Что видит оператор сервера',
    'about.disclaimer': 'Оператор сервера может видеть метаданные трафика: IP-адрес, время подключения и объём переданных данных. Содержимое трафика защищено шифрованием.',
    'about.build': 'Сборка для {os}',
    'about.local': 'Локальное приложение. Без аккаунта и облака.',
    'common.cancel': 'Отмена',
    'common.close': 'Закрыть',
    'common.done': 'Готово',
    'common.loading': 'Загрузка…',
    'common.saving': 'Сохранение…',
    'common.notAvailable': '—',
    'modal.addServer': 'Добавить сервер',
    'modal.addServerSubtitle': 'Параметры туннеля и публичный ключ сервера.',
    'modal.editServer': 'Изменить сервер',
    'modal.editServerSubtitle': 'Обновите параметры профиля.',
    'modal.name': 'Название',
    'modal.namePlaceholder': 'Например, Oracle',
    'modal.host': 'Хост и порт',
    'modal.hostPlaceholder': '1.2.3.4:51820',
    'modal.publicKey': 'Публичный ключ сервера',
    'modal.publicKeyPlaceholder': 'Base64-ключ X25519',
    'modal.tunnelAddress': 'Адрес в туннеле',
    'modal.gateway': 'Шлюз',
    'modal.mtu': 'MTU',
    'modal.privateNote': 'Приватный ключ генерируется локально. Добавьте публичный ключ клиента на сервере — он появится после сохранения.',
    'modal.privateNoteEdit': 'Приватный ключ хранится только на этом устройстве.',
    'modal.add': 'Добавить',
    'modal.save': 'Сохранить',
    'modal.invalidMtu': 'Укажите MTU от 1280 до 1500.',
    'modal.required': 'Заполните обязательные поля.',
    'modal.serverAdded': 'Сервер добавлен',
    'modal.serverUpdated': 'Сервер обновлён',
    'modal.clientKeyTitle': 'Публичный ключ клиента — добавьте его на сервере',
    'modal.clientKeyDesc': 'Откройте конфигурацию сервера W1nWire и добавьте этот ключ в список клиентов. Приватный ключ остаётся на этом устройстве.',
    'modal.clientKeyLabel': 'Публичный ключ клиента',
    'modal.copyKey': 'Копировать ключ',
    'modal.keyCopied': 'Публичный ключ скопирован',
    'modal.importTitle': 'Импорт кода',
    'modal.importSubtitle': 'Вставьте код-приглашение или ссылку W1nWire.',
    'modal.importLabel': 'Код-приглашение',
    'modal.importPlaceholder': 'ww1.... или w1nwire://...',
    'modal.importNote': 'Код содержит адрес сервера и его публичный ключ. Приватный ключ создаётся локально.',
    'modal.importAction': 'Импортировать',
    'modal.imported': 'Сервер импортирован',
    'modal.importEmpty': 'Вставьте код-приглашение.',
    'modal.importInvalid': 'Код не распознан. Проверьте формат.',
    'modal.deleteTitle': 'Удалить сервер?',
    'modal.deleteText': 'Профиль «{name}» будет удалён. Это не повлияет на настройки сервера.',
    'modal.deleteAction': 'Удалить',
    'modal.deleted': 'Сервер удалён',
    'modal.profileNotFound': 'Профиль не найден.',
    'toast.connectError': 'Не удалось подключиться',
    'toast.disconnectError': 'Не удалось отключиться',
    'toast.actionFailed': 'Не удалось выполнить действие',
    'toast.copied': 'Скопировано в буфер обмена',
    'toast.copyFailed': 'Не удалось скопировать',
    'toast.apiUnavailable': 'Нет связи с приложением W1nWire.',
    'toast.noProfiles': 'Сначала добавьте сервер.',
    'toast.testOk': 'Задержка {ms} мс',
    'toast.opened': 'Открыто во внешнем приложении',
    'toast.folderOpened': 'Папка открыта',
    'toast.wintunDone': 'Драйвер Wintun установлен',
    'toast.selftestPassed': 'Самопроверка пройдена',
    'toast.selftestFailed': 'Самопроверка выявила проблемы',
    'toast.relaunch': 'Перезапускаем с правами администратора…',
    'toast.langError': 'Не удалось сохранить язык.'
  },
  en: {
    'nav.aria': 'Main navigation',
    'nav.connect': 'Connection',
    'nav.servers': 'Servers',
    'nav.settings': 'Settings',
    'nav.logs': 'Logs',
    'nav.about': 'About',
    'sidebar.protocol': 'Proprietary protocol',
    'sidebar.version': 'Version',
    'top.noServer': 'No server selected',
    'top.noAddress': 'No address',
    'top.notElevated': 'No administrator rights',
    'top.openServers': 'Open server list',
    'top.language': 'Interface language',
    'state.disconnected': 'Disconnected',
    'state.connecting': 'Connecting…',
    'state.connected': 'Protected',
    'state.reconnecting': 'Reconnecting…',
    'state.disconnecting': 'Disconnecting…',
    'state.error': 'Error',
    'state.eyebrow': 'Status',
    'power.connect': 'Connect',
    'power.disconnect': 'Disconnect',
    'power.cancel': 'Cancel',
    'power.disconnecting': 'Disconnecting…',
    'power.pending': 'Please wait…',
    'caption.disconnected': 'Ready to connect',
    'caption.connecting': 'Establishing a secure channel',
    'caption.connected': 'Tunnel is active',
    'caption.proxy': 'SOCKS5 proxy is active',
    'caption.disconnecting': 'Closing the secure channel',
    'caption.error': 'Check the logs and try again',
    'conn.ip': 'Assigned IP',
    'conn.uptime': 'Session time',
    'conn.fallbackServer': 'No server selected',
    'conn.fallbackAddress': 'No address',
    'conn.error': 'Could not connect. Please try again.',
    'mode.group': 'Connection mode',
    'mode.tunnel': 'Tunnel',
    'mode.proxy': 'Proxy',
    'mode.tunnelDesc': 'All device traffic goes through the encrypted W1nWire tunnel.',
    'mode.proxyDesc': 'Applications use a local SOCKS5 proxy; all other traffic goes direct.',
    'mode.locked': 'Disconnect to change the mode.',
    'mode.proxyAddress': 'Proxy address',
    'mode.systemProxy': 'System proxy',
    'stats.downloaded': 'Downloaded',
    'stats.uploaded': 'Uploaded',
    'stats.speed': 'Speed',
    'stats.latency': 'Latency',
    'stats.session': 'this session',
    'stats.toServer': 'to server',
    'stats.last60': 'Last 60 seconds',
    'stats.rx': 'Receive',
    'stats.tx': 'Send',
    'stats.na': '—',
    'stats.ms': 'ms',
    'notices.adminTitle': 'Tunnel mode requires administrator rights',
    'notices.adminText': 'Relaunch W1nWire as administrator to enable the tunnel.',
    'notices.relaunch': 'Relaunch as administrator',
    'notices.wintunMissing': 'Wintun driver not found',
    'notices.wintunMissingText': 'Tunnel mode is unavailable without it. The file will be downloaded to the app folder.',
    'notices.wintunError': 'Wintun driver error',
    'notices.wintunErrorText': 'Check your internet connection and try again.',
    'notices.wintunDownloading': 'Downloading wintun.dll…',
    'notices.download': 'Download driver (wintun.dll)',
    'notices.retry': 'Retry',
    'notices.wintunVersion': 'Version {version}',
    'empty.eyebrow': 'First run',
    'empty.title': 'Add a server',
    'empty.desc': 'Paste an invite code or add a server manually in the Servers section. Keys are created locally and never leave this device.',
    'empty.invite': 'Paste invite code',
    'empty.free': 'How to get a free server',
    'empty.foot': 'Invite codes start with ww1. or w1nwire://',
    'servers.title': 'Servers',
    'servers.desc': 'Connection profiles. Click a server to select it.',
    'servers.add': 'Add server',
    'servers.import': 'Import code',
    'servers.active': 'Active',
    'servers.connected': 'Connected',
    'servers.connect': 'Connect',
    'servers.disconnect': 'Disconnect',
    'servers.test': 'Test',
    'servers.edit': 'Edit',
    'servers.delete': 'Delete',
    'servers.unnamed': 'Server',
    'servers.activateAria': 'Make server “{name}” active',
    'servers.connectAria': 'Connect to server “{name}”',
    'servers.testAria': 'Test latency for server “{name}”',
    'servers.editAria': 'Edit server “{name}”',
    'servers.deleteAria': 'Delete server “{name}”',
    'servers.emptyTitle': 'No servers yet',
    'servers.emptyDesc': 'Add a profile manually or paste an invite code to get started.',
    'servers.testing': 'Testing…',
    'servers.testFail': 'Could not test the server',
    'settings.title': 'Settings',
    'settings.desc': 'Network, startup and local application data.',
    'settings.group.general': 'General',
    'settings.group.network': 'Network',
    'settings.group.startup': 'Startup & behavior',
    'settings.group.data': 'Data & diagnostics',
    'settings.language': 'Interface language',
    'settings.languageDesc': 'Applies instantly, without a restart.',
    'settings.dns': 'DNS server',
    'settings.dnsDesc': 'Used inside the tunnel. A single IPv4 address.',
    'settings.dnsPlaceholder': '1.1.1.1',
    'settings.mtu': 'MTU',
    'settings.mtuDesc': 'Tunnel packet size. 1420 is recommended.',
    'settings.proxyPort': 'Proxy port',
    'settings.proxyPortDesc': 'Local SOCKS5 port for Proxy mode.',
    'settings.blockIPv6': 'Block IPv6',
    'settings.blockIPv6Desc': 'Prevents IPv6 traffic from leaking outside the tunnel.',
    'settings.autoStart': 'Launch at sign-in',
    'settings.autoStartDesc': 'Start W1nWire when you sign in to Windows.',
    'settings.autoConnect': 'Connect on launch',
    'settings.autoConnectDesc': 'Automatically connect the active profile.',
    'settings.minimizeToTray': 'Minimize to tray',
    'settings.minimizeToTrayDesc': 'Not available in this version.',
    'settings.dataDir': 'Data folder',
    'settings.dataDirDesc': 'Profiles, logs and local keys.',
    'settings.copyPath': 'Copy path',
    'settings.openData': 'Open data folder',
    'settings.openLogs': 'Open logs folder',
    'settings.reset': 'Reset settings',
    'settings.selftest': 'Check installation',
    'settings.selftestRunning': 'Checking…',
    'settings.selftestDesc': 'Checks the driver, privileges, proxy port and configuration files.',
    'settings.invalidValue': 'Check the field value.',
    'settings.resetConfirmTitle': 'Reset settings?',
    'settings.resetConfirmText': 'All settings will return to their defaults. Profiles and logs are kept.',
    'settings.resetDone': 'Settings reset',
    'logs.title': 'Logs',
    'logs.desc': 'Connection, driver and application events. Data stays local.',
    'logs.all': 'All',
    'logs.level.info': 'Info',
    'logs.level.warn': 'Warning',
    'logs.level.error': 'Error',
    'logs.autoScroll': 'Auto-scroll',
    'logs.copy': 'Copy',
    'logs.clear': 'Clear',
    'logs.empty': 'No events yet',
    'logs.emptyFiltered': 'No events at this level',
    'logs.copied': 'Log copied',
    'logs.nothingToCopy': 'There are no events to copy.',
    'logs.clearTitle': 'Clear the log?',
    'logs.clearText': 'All entries will be removed permanently.',
    'logs.copyAria': 'Copy log',
    'logs.clearAria': 'Clear log',
    'logs.cleared': 'Log cleared',
    'about.tagline': 'A personal VPN client for Windows. One executable, a proprietary protocol, and no unnecessary telemetry.',
    'about.version': 'Version {version}',
    'about.protocolLabel': 'Protocol',
    'about.protocol': 'proprietary, X25519 + ChaCha20-Poly1305',
    'about.resources': 'Resources',
    'about.github': 'GitHub',
    'about.docs': 'Documentation',
    'about.license': 'MIT License',
    'about.openExternal': 'Open {name}',
    'about.privacy': 'Privacy',
    'about.privacy.noTelemetry': 'No telemetry: the application never sends data about you anywhere.',
    'about.privacy.noAccounts': 'No accounts and no subscriptions: everything runs on your device.',
    'about.privacy.localKeys': 'Private keys never leave the device and stay in your profile only.',
    'about.privacy.localLogs': 'The event log is stored locally and is cleared with one button.',
    'about.privacy.openSource': 'The code is open under the MIT licence: it can be read in full.',
    'about.disclaimerTitle': 'What the server operator sees',
    'about.disclaimer': 'The server operator can see traffic metadata: IP address, connection time and the amount of data transferred. Traffic contents are protected by encryption.',
    'about.build': 'Build for {os}',
    'about.local': 'Local application. No account or cloud.',
    'common.cancel': 'Cancel',
    'common.close': 'Close',
    'common.done': 'Done',
    'common.loading': 'Loading…',
    'common.saving': 'Saving…',
    'common.notAvailable': '—',
    'modal.addServer': 'Add server',
    'modal.addServerSubtitle': 'Tunnel parameters and the server public key.',
    'modal.editServer': 'Edit server',
    'modal.editServerSubtitle': 'Update this profile.',
    'modal.name': 'Name',
    'modal.namePlaceholder': 'e.g. Oracle',
    'modal.host': 'Host and port',
    'modal.hostPlaceholder': '1.2.3.4:51820',
    'modal.publicKey': 'Server public key',
    'modal.publicKeyPlaceholder': 'X25519 Base64 key',
    'modal.tunnelAddress': 'Tunnel address',
    'modal.gateway': 'Gateway',
    'modal.mtu': 'MTU',
    'modal.privateNote': 'The private key is generated locally. Add the client public key on the server — it will appear after saving.',
    'modal.privateNoteEdit': 'The private key is stored only on this device.',
    'modal.add': 'Add',
    'modal.save': 'Save',
    'modal.invalidMtu': 'Enter an MTU between 1280 and 1500.',
    'modal.required': 'Fill in the required fields.',
    'modal.serverAdded': 'Server added',
    'modal.serverUpdated': 'Server updated',
    'modal.clientKeyTitle': 'Client public key — add it on the server',
    'modal.clientKeyDesc': 'Open the W1nWire server configuration and add this key to the client list. The private key stays on this device.',
    'modal.clientKeyLabel': 'Client public key',
    'modal.copyKey': 'Copy key',
    'modal.keyCopied': 'Public key copied',
    'modal.importTitle': 'Import code',
    'modal.importSubtitle': 'Paste an invite code or W1nWire link.',
    'modal.importLabel': 'Invite code',
    'modal.importPlaceholder': 'ww1.... or w1nwire://...',
    'modal.importNote': 'The code contains the server address and its public key. The private key is created locally.',
    'modal.importAction': 'Import',
    'modal.imported': 'Server imported',
    'modal.importEmpty': 'Paste an invite code.',
    'modal.importInvalid': 'Code not recognised. Check the format.',
    'modal.deleteTitle': 'Delete server?',
    'modal.deleteText': 'Profile “{name}” will be removed. This does not affect the server configuration.',
    'modal.deleteAction': 'Delete',
    'modal.deleted': 'Server deleted',
    'modal.profileNotFound': 'Profile not found.',
    'toast.connectError': 'Could not connect',
    'toast.disconnectError': 'Could not disconnect',
    'toast.actionFailed': 'Action failed',
    'toast.copied': 'Copied to clipboard',
    'toast.copyFailed': 'Could not copy',
    'toast.apiUnavailable': 'Cannot reach the W1nWire application.',
    'toast.noProfiles': 'Add a server first.',
    'toast.testOk': 'Latency {ms} ms',
    'toast.opened': 'Opened in an external application',
    'toast.folderOpened': 'Folder opened',
    'toast.wintunDone': 'Wintun driver installed',
    'toast.selftestPassed': 'Self-test passed',
    'toast.selftestFailed': 'Self-test found problems',
    'toast.relaunch': 'Restarting with administrator rights…',
    'toast.langError': 'Could not save the language.'
  }
};

const ICONS = {
  power: '<path d="M12 3.2v8.2"/><path d="M6.3 6.5a8 8 0 1 0 11.4 0"/>',
  servers: '<rect x="3.5" y="4.5" width="17" height="6" rx="2"/><rect x="3.5" y="13.5" width="17" height="6" rx="2"/><path d="M7 7.5h.01M7 16.5h.01"/>',
  sliders: '<path d="M4 7h9M17 7h3M4 17h3M11 17h9"/><circle cx="15" cy="7" r="2"/><circle cx="9" cy="17" r="2"/>',
  terminal: '<rect x="3.5" y="4.5" width="17" height="15" rx="3"/><path d="m7.5 9 3 3-3 3M13 15h4"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/>',
  shield: '<path d="M12 3 5 6v5c0 4.3 2.8 7.5 7 9 4.2-1.5 7-4.7 7-9V6l-7-3Z"/>',
  shieldCheck: '<path d="M12 3 5 6v5c0 4.3 2.8 7.5 7 9 4.2-1.5 7-4.7 7-9V6l-7-3Z"/><path d="m9 11.8 2 2 4-4"/>',
  chevronDown: '<path d="m7 9.5 5 5 5-5"/>',
  chevronRight: '<path d="m9 6 6 6-6 6"/>',
  copy: '<rect x="8" y="8" width="12" height="12" rx="2.5"/><path d="M16 8V6.5A2.5 2.5 0 0 0 13.5 4h-8A2.5 2.5 0 0 0 3 6.5v8A2.5 2.5 0 0 0 5.5 17H8"/>',
  check: '<path d="m5 12.5 4.5 4.5L19 7.5"/>',
  x: '<path d="M6 6l12 12M18 6 6 18"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  download: '<path d="M12 4v11m0 0 4-4m-4 4-4-4M5 20h14"/>',
  upload: '<path d="M12 20V9m0 0 4 4M12 9 8 13M5 4h14"/>',
  edit: '<path d="M4 20h4.5L19 9.5a2.5 2.5 0 0 0-3.5-3.5L5 16.5V20Z"/><path d="m13.5 7.5 3 3"/>',
  trash: '<path d="M4.5 6.5h15M9 6.5v-2h6v2M6.5 6.5l.8 13h9.4l.8-13M10 10.5v5M14 10.5v5"/>',
  refresh: '<path d="M20 11.5a8 8 0 0 0-14-4.2L4 9.5"/><path d="M4 4.5v5h5"/><path d="M4 12.5a8 8 0 0 0 14 4.2l2-2.2"/><path d="M20 19.5v-5h-5"/>',
  lock: '<rect x="5" y="10" width="14" height="10" rx="2.5"/><path d="M8 10V7.5a4 4 0 0 1 8 0V10"/>',
  external: '<path d="M13 4h7v7M20 4l-9 9"/><path d="M18 14v4a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4"/>',
  folder: '<path d="M3.5 7.5A2.5 2.5 0 0 1 6 5h4l2 2.5h6A2.5 2.5 0 0 1 20.5 10v7A2.5 2.5 0 0 1 18 19.5H6A2.5 2.5 0 0 1 3.5 17V7.5Z"/>',
  alert: '<path d="M12 3.8 21 20H3L12 3.8Z"/><path d="M12 9v5M12 17h.01"/>',
  alertCircle: '<circle cx="12" cy="12" r="9"/><path d="M12 7.5v5M12 16.5h.01"/>',
  checkCircle: '<circle cx="12" cy="12" r="9"/><path d="m8 12.2 2.7 2.7 5.4-5.4"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7.5V12l3 2"/>',
  activity: '<path d="M3 12h4l2.5-6.5 4 13L16.5 12H21"/>',
  monitor: '<rect x="3" y="4.5" width="18" height="12" rx="2.5"/><path d="M9 20h6M12 16.5V20"/>',
  key: '<circle cx="8" cy="15.5" r="3.5"/><path d="m10.5 13 8-8M16 5l3 3M14 7l2.5 2.5"/>',
  book: '<path d="M4 5.5A2.5 2.5 0 0 1 6.5 3H20v16H6.5A2.5 2.5 0 0 0 4 21.5v-16Z"/><path d="M4 18.5A2.5 2.5 0 0 1 6.5 16H20"/>',
  github: '<path d="M9 19c-4 1.2-4-2-6-2.5M15 21v-3.4a3 3 0 0 0-.8-2.3c2.8-.3 5.8-1.4 5.8-6a4.7 4.7 0 0 0-1.3-3.2 4.2 4.2 0 0 0-.1-3.2s-1-.3-3.3 1.2a11.4 11.4 0 0 0-5.8 0C7.2 2.6 6.2 2.9 6.2 2.9a4.2 4.2 0 0 0-.1 3.2A4.7 4.7 0 0 0 4.8 9.3c0 4.6 3 5.7 5.8 6a3 3 0 0 0-.8 2.3V21"/>',
  scale: '<path d="M12 4v16M7 20h10M5 8h14M8 8l-3 6a3 3 0 0 0 6 0L8 8ZM16 8l-3 6a3 3 0 0 0 6 0l-3-6Z"/>',
  wifi: '<path d="M3.5 9a13 13 0 0 1 17 0M6.5 12.5a8.5 8.5 0 0 1 11 0M9.5 16a4 4 0 0 1 5 0"/><circle cx="12" cy="19" r=".8" fill="currentColor" stroke="none"/>',
  serverPlus: '<rect x="3.5" y="4" width="13" height="6" rx="2"/><rect x="3.5" y="14" width="13" height="6" rx="2"/><path d="M7 7h.01M7 17h.01M20 7v4M18 9h4M20 15v5M17.5 17.5h5"/>',
  arrowUpRight: '<path d="M7 17 17 7M8 7h9v9"/>',
  downloadCloud: '<path d="M7 17.5a4 4 0 0 1-.7-7.94A5.5 5.5 0 0 1 17.2 8.6 4.5 4.5 0 0 1 17 17.5"/><path d="M12 11v9m0 0 3.5-3.5M12 20l-3.5-3.5"/>'
};

const URLS = {
  github: 'https://github.com/w1nchi/w1nwire',
  docs: 'https://github.com/w1nchi/w1nwire#readme',
  license: 'https://github.com/w1nchi/w1nwire/blob/main/LICENSE',
  free: 'https://github.com/w1nchi/w1nwire/blob/main/docs/ORACLE.md'
};

const DEFAULT_SETTINGS = Object.freeze({
  language: 'ru',
  dns: '1.1.1.1',
  mtu: 1420,
  proxyPort: 1080,
  blockIPv6: true,
  autoStart: false,
  autoConnect: false,
  minimizeToTray: false,
  systemProxy: false
});

const STATES = ['disconnected', 'connecting', 'connected', 'reconnecting', 'disconnecting', 'error'];
const LEVELS = ['info', 'warn', 'error'];
const PROFILE_DEFAULTS = {
  tunnelAddress: '10.66.0.2/24',
  gateway: '10.66.0.1',
  mtu: 1420
};

/* ---------- tiny helpers ---------- */

const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));

function esc(value) {
  return String(value == null ? '' : value).replace(/[&<>"']/g, (ch) => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;'
  }[ch]));
}

function str(value, fallback = '') {
  if (typeof value === 'string') return value;
  if (value == null) return fallback;
  return String(value);
}

function num(value, fallback = 0) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function positiveNum(value, fallback) {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}

function clamp(value, min, max) {
  return Math.min(max, Math.max(min, value));
}

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function t(key, vars) {
  const dict = I18N[UI.lang] || I18N.ru;
  let text = dict[key];
  if (text == null) text = I18N.ru[key];
  if (text == null) text = key;
  if (vars) {
    Object.keys(vars).forEach((name) => {
      text = text.replace(new RegExp('\\{' + name + '\\}', 'g'), () => String(vars[name]));
    });
  }
  return text;
}

function icon(name, extraClass = '') {
  const body = ICONS[name] || ICONS.info;
  return '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"' +
    (extraClass ? ' class="' + esc(extraClass) + '"' : '') + '>' + body + '</svg>';
}

function formatNumber(value, digits) {
  return Number(value).toLocaleString(UI.lang === 'ru' ? 'ru-RU' : 'en-US', {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits
  });
}

function fmtBytes(value) {
  let n = Math.max(0, num(value, 0));
  const units = UI.lang === 'ru' ? ['Б', 'КБ', 'МБ', 'ГБ', 'ТБ'] : ['B', 'KB', 'MB', 'GB', 'TB'];
  let index = 0;
  while (n >= 1024 && index < units.length - 1) {
    n /= 1024;
    index += 1;
  }
  let digits = 0;
  if (index > 0) digits = n >= 100 ? 0 : n >= 10 ? 1 : 2;
  return formatNumber(n, digits) + ' ' + units[index];
}

function fmtRate(value) {
  return fmtBytes(value) + (UI.lang === 'ru' ? '/с' : '/s');
}

function fmtDuration(totalSeconds) {
  const safe = Math.max(0, Math.floor(num(totalSeconds, 0)));
  const hours = Math.floor(safe / 3600);
  const minutes = Math.floor((safe % 3600) / 60);
  const seconds = safe % 60;
  return [hours, minutes, seconds].map((part) => String(part).padStart(2, '0')).join(':');
}

function fmtLatency(value) {
  if (value == null) return t('stats.na');
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) return t('stats.na');
  return Math.round(parsed) + ' ' + t('stats.ms');
}

function fmtChipLatency(value) {
  if (value == null) return t('stats.na') + ' ' + t('stats.ms');
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) return t('stats.na') + ' ' + t('stats.ms');
  return Math.round(parsed) + ' ' + t('stats.ms');
}

function latencyTone(value) {
  if (value == null) return '';
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return '';
  if (parsed <= 60) return 'is-good';
  if (parsed <= 120) return 'is-mid';
  return 'is-bad';
}

function stateLabel(state) {
  const key = 'state.' + (STATES.includes(state) ? state : 'disconnected');
  return t(key);
}

function stateTone(state) {
  if (state === 'connected') return 'connected';
  if (state === 'connecting' || state === 'reconnecting') return 'connecting';
  if (state === 'disconnecting') return 'disconnecting';
  if (state === 'error') return 'error';
  return 'disconnected';
}

function currentTime() {
  try {
    return new Date().toLocaleTimeString(UI.lang === 'ru' ? 'ru-RU' : 'en-US', { hour12: false });
  } catch (error) {
    return '--:--:--';
  }
}

function isEditingTarget(target) {
  if (!target || !(target instanceof Element)) return false;
  return Boolean(target.closest('input, textarea, select, [contenteditable="true"]'));
}

function setText(selector, value) {
  $$(selector).forEach((element) => {
    element.textContent = value;
  });
}

/* ---------- state normalisation ---------- */

function normalizeSettings(raw) {
  const source = raw && typeof raw === 'object' ? raw : {};
  return {
    language: source.language === 'en' ? 'en' : 'ru',
    dns: str(source.dns, DEFAULT_SETTINGS.dns),
    mtu: positiveNum(source.mtu, DEFAULT_SETTINGS.mtu),
    proxyPort: positiveNum(source.proxyPort, DEFAULT_SETTINGS.proxyPort),
    blockIPv6: source.blockIPv6 !== false,
    autoStart: Boolean(source.autoStart),
    autoConnect: Boolean(source.autoConnect),
    minimizeToTray: Boolean(source.minimizeToTray),
    systemProxy: Boolean(source.systemProxy)
  };
}

function normalizeStatus(raw) {
  const source = raw && typeof raw === 'object' ? raw : {};
  const state = STATES.includes(source.state) ? source.state : 'disconnected';
  return {
    state,
    connected: state === 'connected',
    since: source.since ? str(source.since) : null,
    uptimeSec: Math.max(0, num(source.uptimeSec, 0)),
    server: str(source.server),
    profileName: str(source.profileName),
    mode: source.mode === 'proxy' ? 'proxy' : 'tunnel',
    assignedIP: str(source.assignedIP),
    rxBytes: Math.max(0, num(source.rxBytes, 0)),
    txBytes: Math.max(0, num(source.txBytes, 0)),
    rxRate: Math.max(0, num(source.rxRate, 0)),
    txRate: Math.max(0, num(source.txRate, 0)),
    rttMs: source.rttMs == null ? null : num(source.rttMs, null),
    error: str(source.error),
    selfTest: Boolean(source.selfTest)
  };
}

function normalizeProfile(raw, index) {
  const source = raw && typeof raw === 'object' ? raw : {};
  return {
    id: str(source.id) || 'profile-' + index,
    name: str(source.name).trim(),
    host: str(source.host),
    publicKey: str(source.publicKey),
    privateKeySet: Boolean(source.privateKeySet),
    tunnelAddress: str(source.tunnelAddress, PROFILE_DEFAULTS.tunnelAddress),
    gateway: str(source.gateway, PROFILE_DEFAULTS.gateway),
    mtu: positiveNum(source.mtu, PROFILE_DEFAULTS.mtu),
    createdAt: str(source.createdAt),
    latencyMs: source.latencyMs == null ? null : num(source.latencyMs, null)
  };
}

function normalizeLog(raw) {
  const source = raw && typeof raw === 'object' ? raw : {};
  const level = LEVELS.includes(str(source.level).toLowerCase()) ? str(source.level).toLowerCase() : 'info';
  return {
    t: str(source.t) || currentTime(),
    level,
    msg: str(source.msg)
  };
}

function normalizeState(raw) {
  const source = raw && typeof raw === 'object' ? raw : {};
  const wintunSource = source.wintun && typeof source.wintun === 'object' ? source.wintun : {};
  const wintunStatus = ['ready', 'missing', 'error'].includes(wintunSource.status) ? wintunSource.status : 'ready';
  const profiles = Array.isArray(source.profiles)
    ? source.profiles.filter((item) => item && typeof item === 'object').map(normalizeProfile)
    : [];
  const logs = Array.isArray(source.logs)
    ? source.logs.filter((item) => item && typeof item === 'object').map(normalizeLog)
    : [];
  return {
    version: str(source.version, '2.0.0'),
    os: str(source.os, 'windows'),
    elevated: Boolean(source.elevated),
    dataDir: str(source.dataDir),
    wintun: {
      status: wintunStatus,
      version: str(wintunSource.version),
      detail: str(wintunSource.detail)
    },
    status: normalizeStatus(source.status),
    profiles,
    activeProfileId: str(source.activeProfileId),
    settings: normalizeSettings(source.settings),
    logs: logs.slice(-1000),
    busy: str(source.busy)
  };
}

function defaultState() {
  return normalizeState({
    version: '2.0.0',
    os: 'windows',
    elevated: false,
    dataDir: '',
    wintun: { status: 'ready', version: '', detail: '' },
    status: { state: 'disconnected', connected: false, mode: 'tunnel' },
    profiles: [],
    activeProfileId: '',
    settings: { ...DEFAULT_SETTINGS },
    logs: [],
    busy: ''
  });
}

function activeProfile() {
  const byId = appState.profiles.find((profile) => profile.id === appState.activeProfileId);
  return byId || appState.profiles[0] || null;
}

function currentUptime() {
  const status = appState.status;
  if (UI.mock) {
    return status.state === 'connected' ? status.uptimeSec : 0;
  }
  if (status.state === 'connected') {
    return Math.max(status.uptimeSec || 0, UI.uptimeBase + Math.floor((performance.now() - UI.uptimeAt) / 1000));
  }
  if (status.state === 'reconnecting' || status.state === 'disconnecting') return status.uptimeSec || 0;
  return 0;
}

/* ---------- mock mode ---------- */

const UI = {
  mock: false,
  view: 'connect',
  initialView: 'connect',
  lang: 'ru',
  mode: 'tunnel',
  modeTouched: false,
  pending: false,
  testingProfileId: '',
  logFilter: 'all',
  autoScroll: true,
  uptimeBase: 0,
  uptimeAt: performance.now(),
  spark: { rx: new Array(60).fill(0), tx: new Array(60).fill(0) },
  modal: null,
  lastFocus: null,
  lastAnnouncedState: '',
  mockToken: 0,
  tickStarted: false,
  pollTimer: null,
  eventSource: null
};

let appState = defaultState();

function detectMock() {
  try {
    const params = new URLSearchParams(location.search);
    return params.get('mock') === '1' || location.hash === '#mock' || location.hash.startsWith('#mock?');
  } catch (error) {
    return location.hash === '#mock';
  }
}

function mockParams() {
  try {
    return new URLSearchParams(location.search);
  } catch (error) {
    return new URLSearchParams('');
  }
}

function makeMockPublicKey(seed) {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
  let output = '';
  const base = String(seed || 'w1nwire');
  for (let index = 0; index < 43; index += 1) {
    output += alphabet[(base.charCodeAt(index % base.length) + index * 17) % alphabet.length];
  }
  return output + '=';
}

function mockNowTime(offsetSeconds) {
  const date = new Date(Date.now() - (offsetSeconds || 0) * 1000);
  return date.toLocaleTimeString('ru-RU', { hour12: false });
}

function createMockState() {
  const params = mockParams();
  const profiles = [
    {
      id: 'p1',
      name: 'Oracle',
      host: '1.2.3.4:51820',
      publicKey: makeMockPublicKey('oracle-server-key'),
      privateKeySet: true,
      tunnelAddress: '10.66.0.2/24',
      gateway: '10.66.0.1',
      mtu: 1420,
      createdAt: '2026-08-02T18:14:00Z',
      latencyMs: 28
    },
    {
      id: 'p2',
      name: 'Frankfurt',
      host: '5.9.12.44:51820',
      publicKey: makeMockPublicKey('frankfurt-server-key'),
      privateKeySet: true,
      tunnelAddress: '10.66.0.6/24',
      gateway: '10.66.0.1',
      mtu: 1420,
      createdAt: '2026-09-17T09:31:00Z',
      latencyMs: 67
    }
  ];
  const state = {
    version: '2.0.0',
    os: 'windows',
    elevated: false,
    dataDir: 'C:\\Users\\W1nchi\\AppData\\Roaming\\W1nWire',
    wintun: { status: 'ready', version: '0.14.1', detail: '' },
    status: {
      state: 'disconnected',
      connected: false,
      since: null,
      uptimeSec: 0,
      server: '1.2.3.4:51820',
      profileName: 'Oracle',
      mode: 'tunnel',
      assignedIP: '',
      rxBytes: 0,
      txBytes: 0,
      rxRate: 0,
      txRate: 0,
      rttMs: null,
      error: '',
      selfTest: false
    },
    profiles,
    activeProfileId: 'p1',
    settings: { ...DEFAULT_SETTINGS },
    logs: [
      { t: mockNowTime(214), level: 'info', msg: 'W1nWire 2.0.0 запущен' },
      { t: mockNowTime(208), level: 'info', msg: 'Загружены локальные ключи профиля Oracle' },
      { t: mockNowTime(203), level: 'info', msg: 'Драйвер Wintun 0.14.1 готов' },
      { t: mockNowTime(176), level: 'warn', msg: 'Права администратора не предоставлены, доступен режим прокси' },
      { t: mockNowTime(98), level: 'info', msg: 'Конфигурация сервера 1.2.3.4:51820 проверена' },
      { t: mockNowTime(94), level: 'error', msg: 'Первая попытка рукопожатия завершилась таймаутом' },
      { t: mockNowTime(91), level: 'info', msg: 'Повторное рукопожатие X25519 завершено' },
      { t: mockNowTime(90), level: 'info', msg: 'Туннель установлен, локальный адрес 10.66.0.2' }
    ],
    busy: ''
  };

  if (params.get('lang') === 'en') state.settings.language = 'en';
  if (params.get('elevated') === '1' || params.get('admin') === '1') state.elevated = true;
  if (params.get('wintun') === 'missing') {
    state.wintun = { status: 'missing', version: '', detail: 'wintun.dll not found next to w1nwire.exe' };
  }
  if (params.get('wintun') === 'error') {
    state.wintun = { status: 'error', version: '', detail: 'Access is denied' };
  }
  if (params.get('profiles') === '0') {
    state.profiles = [];
    state.activeProfileId = '';
    state.status.server = '';
    state.status.profileName = '';
    state.status.state = 'disconnected';
  }
  if (params.get('mode') === 'proxy') {
    state.status.mode = 'proxy';
  }

  const requestedState = params.get('state') || params.get('mockState');
  if (requestedState && STATES.includes(requestedState) && state.profiles.length) {
    state.status.state = requestedState;
    state.status.connected = requestedState === 'connected';
    if (requestedState === 'connected') {
      state.status.since = new Date(Date.now() - 754 * 1000).toISOString();
      state.status.uptimeSec = 754;
      state.status.assignedIP = '10.66.0.2';
      state.status.rxBytes = 1384120320;
      state.status.txBytes = 88473600;
      state.status.rxRate = 2485120;
      state.status.txRate = 147456;
      state.status.rttMs = 27;
    } else if (requestedState === 'connecting' || requestedState === 'reconnecting') {
      state.status.assignedIP = '';
      state.status.rttMs = null;
    } else if (requestedState === 'error') {
      state.status.assignedIP = '';
      state.status.error = 'Рукопожатие не завершено: сервер не отвечает';
      state.status.rttMs = null;
    }
  }

  const requestedView = params.get('view');
  if (['connect', 'servers', 'settings', 'logs', 'about'].includes(requestedView)) {
    UI.initialView = requestedView;
    UI.view = requestedView;
  }
  return state;
}

function mockLog(level, msg) {
  appState.logs.push({ t: currentTime(), level, msg });
  if (appState.logs.length > 1000) appState.logs = appState.logs.slice(-1000);
}

function setMockStatus(nextState, extra) {
  UI.mockToken += 1;
  const profile = activeProfile();
  const base = {
    ...appState.status,
    ...(extra || {}),
    state: nextState,
    connected: nextState === 'connected',
    mode: UI.mode,
    profileName: profile ? profile.name : appState.status.profileName,
    server: profile ? profile.host : appState.status.server,
    error: nextState === 'error' ? (extra && extra.error) || 'Рукопожатие не завершено: сервер не отвечает' : '',
    rttMs: nextState === 'connected' ? (extra && extra.rttMs) || 27 : null,
    assignedIP: nextState === 'connected' ? '10.66.0.2' : ''
  };
  if (nextState === 'connected') {
    base.since = new Date().toISOString();
    base.uptimeSec = 0;
    base.rxRate = 0;
    base.txRate = 0;
  } else {
    base.since = null;
    base.uptimeSec = 0;
    base.rxRate = 0;
    base.txRate = 0;
  }
  appState.status = normalizeStatus(base);
  UI.uptimeBase = base.uptimeSec;
  UI.uptimeAt = performance.now();
}

function scheduleMockConnected(token) {
  setTimeout(() => {
    if (token !== UI.mockToken) return;
    if (appState.status.state !== 'connecting') return;
    setMockStatus('connected');
    setMockStatusRate();
    mockLog('info', 'Туннель установлен, локальный адрес 10.66.0.2');
    renderAll();
  }, 1350);
}

function setMockStatusRate() {
  appState.status.rxRate = 1884000;
  appState.status.txRate = 96300;
}

function cycleMockState() {
  if (!UI.mock) return;
  const order = ['disconnected', 'connecting', 'connected', 'error'];
  const current = order.indexOf(appState.status.state);
  const next = order[(current + 1) % order.length];
  setMockStatus(next);
  if (next === 'connected') {
    appState.status.rxBytes = Math.max(appState.status.rxBytes, 146800640);
    appState.status.txBytes = Math.max(appState.status.txBytes, 10485760);
    mockLog('info', 'Туннель установлен, локальный адрес 10.66.0.2');
  } else if (next === 'connecting') {
    mockLog('info', 'Подключение к ' + (activeProfile() ? activeProfile().host : 'серверу'));
  } else if (next === 'error') {
    mockLog('error', 'Рукопожатие не завершено: сервер не отвечает');
  } else {
    mockLog('info', 'Соединение закрыто');
  }
  renderAll();
}

/* ---------- API bridge ---------- */

// api calls the local backend. The optional context is prepended to the
// error message, so that a failure says what was being attempted and not
// only what the server answered.
async function api(path, payload, context) {
  try {
    if (UI.mock) return await mockApi(path, payload || {});
    const response = await fetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload || {})
    });
    let data = null;
    try {
      data = await response.json();
    } catch (error) {
      data = null;
    }
    if (!response.ok || (data && data.ok === false)) {
      const message = data && data.error ? String(data.error) : 'HTTP ' + response.status;
      const apiError = new Error(message);
      apiError.handled = false;
      throw apiError;
    }
    return data || { ok: true };
  } catch (error) {
    const isNetworkError = error instanceof TypeError || !error || !error.message;
    const detail = isNetworkError ? t('toast.apiUnavailable') : String(error.message);
    showToast('error', context ? context + ': ' + detail : detail);
    if (error && typeof error === 'object') error.handled = true;
    throw error;
  }
}

async function mockApi(path, payload) {
  await delay(120);
  const params = payload || {};
  switch (path) {
    case '/api/connect': {
      const profile = appState.profiles.find((item) => item.id === params.profileId) || activeProfile();
      if (!profile) throw new Error(t('toast.noProfiles'));
      appState.activeProfileId = profile.id;
      UI.mode = params.mode === 'proxy' ? 'proxy' : 'tunnel';
      UI.modeTouched = true;
      setMockStatus('connecting');
      appState.status.profileName = profile.name;
      appState.status.server = profile.host;
      mockLog('info', 'Подключение к ' + profile.host);
      renderAll();
      const token = UI.mockToken;
      scheduleMockConnected(token);
      return { ok: true };
    }
    case '/api/disconnect': {
      setMockStatus('disconnected');
      mockLog('info', 'Соединение закрыто');
      renderAll();
      return { ok: true };
    }
    case '/api/profiles/add': {
      const profile = normalizeProfile({
        id: 'p' + (appState.profiles.length + 1) + '-' + Date.now().toString(36),
        name: params.name,
        host: params.host,
        publicKey: params.publicKey,
        privateKeySet: true,
        tunnelAddress: params.tunnelAddress,
        gateway: params.gateway,
        mtu: params.mtu,
        createdAt: new Date().toISOString(),
        latencyMs: null
      }, appState.profiles.length);
      appState.profiles.push(profile);
      if (!appState.activeProfileId) appState.activeProfileId = profile.id;
      mockLog('info', 'Добавлен профиль ' + (profile.name || profile.host));
      return { ok: true, profile, publicKey: makeMockPublicKey(profile.name + profile.host) };
    }
    case '/api/profiles/update': {
      const index = appState.profiles.findIndex((item) => item.id === params.id);
      if (index < 0) throw new Error(t('modal.profileNotFound'));
      const current = appState.profiles[index];
      const profile = normalizeProfile({
        ...current,
        ...params,
        id: current.id,
        privateKeySet: current.privateKeySet,
        createdAt: current.createdAt,
        latencyMs: current.latencyMs
      }, index);
      appState.profiles[index] = profile;
      mockLog('info', 'Профиль ' + (profile.name || profile.host) + ' обновлён');
      return { ok: true, profile };
    }
    case '/api/profiles/delete': {
      const profile = appState.profiles.find((item) => item.id === params.id);
      appState.profiles = appState.profiles.filter((item) => item.id !== params.id);
      if (appState.activeProfileId === params.id) {
        appState.activeProfileId = appState.profiles[0] ? appState.profiles[0].id : '';
      }
      mockLog('info', 'Профиль ' + (profile ? profile.name : '') + ' удалён');
      return { ok: true };
    }
    case '/api/profiles/activate': {
      if (!appState.profiles.some((item) => item.id === params.id)) throw new Error(t('modal.profileNotFound'));
      appState.activeProfileId = params.id;
      return { ok: true };
    }
    case '/api/profiles/import': {
      const code = str(params.code).trim();
      if (!code) throw new Error(t('modal.importEmpty'));
      if (!code.startsWith('ww1.') && !code.startsWith('w1nwire://')) {
        throw new Error(t('modal.importInvalid'));
      }
      const profile = normalizeProfile({
        id: 'imp-' + Date.now().toString(36),
        name: 'W1nWire Free',
        host: '185.24.10.8:51820',
        publicKey: makeMockPublicKey(code),
        privateKeySet: true,
        tunnelAddress: '10.66.0.2/24',
        gateway: '10.66.0.1',
        mtu: 1420,
        createdAt: new Date().toISOString(),
        latencyMs: null
      }, appState.profiles.length);
      appState.profiles.push(profile);
      if (!appState.activeProfileId) appState.activeProfileId = profile.id;
      mockLog('info', 'Импортирован профиль W1nWire Free');
      return { ok: true, profile };
    }
    case '/api/profiles/test': {
      const profile = appState.profiles.find((item) => item.id === params.id);
      if (!profile) throw new Error(t('modal.profileNotFound'));
      const rtt = 18 + Math.floor(Math.random() * 54);
      profile.latencyMs = rtt;
      return { ok: true, rttMs: rtt };
    }
    case '/api/settings': {
      appState.settings = normalizeSettings({ ...appState.settings, ...params });
      if (params.language) UI.lang = appState.settings.language;
      return { ok: true };
    }
    case '/api/system-proxy': {
      appState.settings.systemProxy = Boolean(params.enabled);
      return { ok: true };
    }
    case '/api/open-external':
      return { ok: true };
    case '/api/open-folder':
      return { ok: true };
    case '/api/wintun/install': {
      appState.busy = 'wintun-download';
      renderView();
      renderTopbar();
      await delay(1700);
      appState.wintun = { status: 'ready', version: '0.14.1', detail: '' };
      appState.busy = '';
      mockLog('info', 'Драйвер Wintun 0.14.1 установлен');
      return { ok: true };
    }
    case '/api/relaunch-admin': {
      appState.elevated = true;
      mockLog('info', 'Приложение перезапущено с правами администратора');
      return { ok: true };
    }
    case '/api/selftest': {
      appState.busy = 'self-test';
      renderView();
      renderTopbar();
      await delay(1400);
      appState.busy = '';
      appState.status.selfTest = true;
      mockLog('info', 'Самопроверка: драйвер, права, порт 1080 и конфигурация в порядке');
      return { ok: true };
    }
    case '/api/logs/clear': {
      appState.logs = [];
      return { ok: true };
    }
    default:
      return { ok: true };
  }
}

/* ---------- startup ---------- */

async function init() {
  UI.mock = detectMock();
  hydrateStaticIcons();
  bindEvents();

  if (UI.mock) {
    appState = normalizeState(createMockState());
    UI.view = UI.initialView;
    UI.lang = appState.settings.language;
    UI.mode = appState.status.mode;
    UI.modeTouched = appState.status.state === 'connected';
    UI.uptimeBase = appState.status.uptimeSec;
    UI.uptimeAt = performance.now();
    if (appState.status.state === 'connected') seedMockSpark();
    renderAll();
    applyMockModalQuery();
    startTicker();
    return;
  }

  try {
    const raw = await fetchState();
    appState = normalizeState(raw);
  } catch (error) {
    appState = defaultState();
    showToast('error', t('toast.apiUnavailable'));
  }

  UI.lang = appState.settings.language;
  UI.mode = appState.status.mode;
  UI.uptimeBase = appState.status.uptimeSec;
  UI.uptimeAt = performance.now();
  renderAll();
  setupEvents();
  UI.pollTimer = setInterval(() => {
    if (!document.hidden) refreshState(true);
  }, 10000);
  startTicker();
}

async function fetchState() {
  const response = await fetch('/api/state', { cache: 'no-store' });
  if (!response.ok) throw new Error('HTTP ' + response.status);
  return response.json();
}

async function refreshState(soft) {
  try {
    const raw = await fetchState();
    applyFullState(raw, false);
  } catch (error) {
    if (!soft) showToast('error', t('toast.apiUnavailable'));
  }
}

function applyFullState(raw, initial) {
  const next = normalizeState(raw);
  const languageChanged = next.settings.language !== UI.lang;
  const previousState = appState.status.state;
  appState = next;
  if (initial || languageChanged) {
    UI.lang = next.settings.language;
    updateStaticText();
  }
  if (!UI.modeTouched || next.status.state === 'connected') {
    UI.mode = next.status.mode;
  }
  UI.uptimeBase = next.status.uptimeSec;
  UI.uptimeAt = performance.now();
  renderNav();
  renderTopbar(initial);
  if (next.status.state !== previousState) announceState(next.status.state);
  if (!initial && (isEditingTarget(document.activeElement) || UI.modal)) {
    updateLiveDom();
    return;
  }
  renderView();
}

function seedMockSpark() {
  const rx = [];
  const tx = [];
  for (let index = 0; index < 60; index += 1) {
    const wave = 0.62 + 0.24 * Math.sin(index / 4.1) + 0.12 * Math.sin(index / 1.7);
    const dip = index % 17 === 0 ? 0.42 : 1;
    rx.push(Math.max(160000, Math.round((1320000 + wave * 1180000 + (index % 5) * 26000) * dip)));
    tx.push(Math.max(12000, Math.round(92000 + (0.5 + 0.36 * Math.sin(index / 5.6)) * 98000)));
  }
  UI.spark = { rx, tx };
}

function applyMockModalQuery() {
  if (!UI.mock) return;
  const requested = mockParams().get('modal');
  if (requested === 'add') openProfileModal('');
  else if (requested === 'edit') openProfileModal('p1');
  else if (requested === 'import') openImportModal();
  else if (requested === 'key') openClientKeyModal(makeMockPublicKey('client-demo'));
  else if (requested === 'confirm') openDeleteConfirm('p1');
}

function startTicker() {
  if (UI.tickStarted) return;
  UI.tickStarted = true;
  setInterval(tick, 1000);
  window.addEventListener('resize', () => drawChart());
}

function tick() {
  const status = appState.status;
  if (UI.mock && status.state === 'connected') {
    status.uptimeSec += 1;
    const wave = Math.sin(Date.now() / 7600);
    status.rxRate = Math.max(140000, Math.round(1680000 + wave * 640000 + Math.random() * 460000));
    status.txRate = Math.max(12000, Math.round(118000 + Math.random() * 92000));
    status.rxBytes += status.rxRate;
    status.txBytes += status.txRate;
    status.rttMs = 23 + Math.round(Math.random() * 11);
    if (!status.assignedIP) status.assignedIP = '10.66.0.2';
  }
  const rxRate = status.connected ? status.rxRate : 0;
  const txRate = status.connected ? status.txRate : 0;
  UI.spark.rx.push(Math.max(0, rxRate));
  UI.spark.tx.push(Math.max(0, txRate));
  if (UI.spark.rx.length > 60) UI.spark.rx.shift();
  if (UI.spark.tx.length > 60) UI.spark.tx.shift();
  updateLiveDom();
  drawChart();
}

/* ---------- SSE ---------- */

function setupEvents() {
  if (typeof EventSource === 'undefined') return;
  try {
    UI.eventSource = new EventSource('/api/events');
  } catch (error) {
    return;
  }
  UI.eventSource.addEventListener('state', (event) => {
    try {
      const data = JSON.parse(event.data);
      const previous = appState.status.state;
      appState.status = normalizeStatus(data);
      UI.uptimeBase = appState.status.uptimeSec;
      UI.uptimeAt = performance.now();
      if (appState.status.state !== previous && appState.status.state === 'connected') {
        UI.mode = appState.status.mode;
      }
      announceState(appState.status.state);
      softRefresh();
    } catch (error) {
      /* malformed event: polling remains the fallback */
    }
  });
  UI.eventSource.addEventListener('traffic', (event) => {
    try {
      const data = JSON.parse(event.data);
      const status = appState.status;
      ['rxBytes', 'txBytes', 'rxRate', 'txRate', 'uptimeSec'].forEach((key) => {
        if (data[key] != null && Number.isFinite(Number(data[key]))) {
          status[key] = Math.max(0, Number(data[key]));
        }
      });
      UI.uptimeBase = status.uptimeSec;
      UI.uptimeAt = performance.now();
      updateLiveDom();
    } catch (error) {
      /* ignore */
    }
  });
  UI.eventSource.addEventListener('log', (event) => {
    try {
      const entry = normalizeLog(JSON.parse(event.data));
      appState.logs.push(entry);
      if (appState.logs.length > 1000) appState.logs = appState.logs.slice(-1000);
      if (UI.view === 'logs') refreshLogsView();
    } catch (error) {
      /* ignore */
    }
  });
  UI.eventSource.addEventListener('busy', (event) => {
    try {
      const data = JSON.parse(event.data);
      const previous = appState.busy;
      appState.busy = str(data.busy);
      softRefresh();
      if (previous === 'wintun-download' && !appState.busy) {
        setTimeout(() => refreshState(true), 700);
      }
    } catch (error) {
      /* ignore */
    }
  });
  // The server pushes the profile list whenever it changes (add, edit,
  // delete, activation, a new latency measurement). Without this the list
  // would only refresh on the ten second poll.
  UI.eventSource.addEventListener('profiles', (event) => {
    try {
      const data = JSON.parse(event.data);
      if (!Array.isArray(data)) return;
      appState.profiles = data
        .filter((item) => item && typeof item === 'object')
        .map(normalizeProfile);
      if (!appState.profiles.some((profile) => profile.id === appState.activeProfileId)) {
        appState.activeProfileId = appState.profiles.length ? appState.profiles[0].id : '';
      }
      softRefresh();
    } catch (error) {
      /* ignore */
    }
  });
  // The log was emptied somewhere else (the button in another window, a
  // remote call), so the rows on screen have to go too.
  UI.eventSource.addEventListener('logs-cleared', () => {
    appState.logs = [];
    if (UI.view === 'logs') renderView();
  });
  UI.eventSource.onerror = () => {
    /* EventSource reconnects automatically; polling covers longer outages. */
  };
}

function softRefresh() {
  if (!(document.activeElement && document.activeElement.closest && document.activeElement.closest('.topbar'))) {
    renderTopbar();
  }
  if (UI.modal || isEditingTarget(document.activeElement)) {
    updateLiveDom();
    return;
  }
  renderView();
  updateLiveDom();
}

/* ============================================================
   Rendering
   ============================================================ */

function hydrateStaticIcons() {
  $$('[data-icon]').forEach((element) => {
    element.innerHTML = icon(element.dataset.icon);
  });
}

function updateStaticText() {
  document.documentElement.lang = UI.lang;
  $$('[data-i18n]').forEach((element) => {
    const key = element.dataset.i18n;
    if (key) element.textContent = t(key);
  });
  const nav = $('#mainNav');
  if (nav) nav.setAttribute('aria-label', t('nav.aria'));
  const langGroup = document.querySelector('.lang-switch');
  if (langGroup) langGroup.setAttribute('aria-label', t('top.language'));
  updateDocumentTitle();
}

// updateDocumentTitle puts the current screen in the title bar of the
// window, so that a taskbar with many windows still tells them apart.
function updateDocumentTitle() {
  const label = t('nav.' + UI.view);
  document.title = label ? 'W1nWire — ' + label : 'W1nWire';
}

function renderAll() {
  updateStaticText();
  renderNav();
  renderTopbar(true);
  renderView();
  const version = $('#appVersion');
  if (version) version.textContent = appState.version || '2.0.0';
}

function renderNav() {
  $$('#mainNav .nav-item').forEach((item) => {
    const active = item.dataset.view === UI.view;
    item.classList.toggle('is-active', active);
    if (active) item.setAttribute('aria-current', 'page');
    else item.removeAttribute('aria-current');
  });
}

function renderTopbar(force) {
  const active = document.activeElement;
  if (!force && active && active.closest && active.closest('.topbar')) return;
  const host = $('#topbarInner');
  if (!host) return;
  const profile = activeProfile();
  const status = appState.status;
  const name = status.profileName || (profile ? profile.name : '') || t('top.noServer');
  const address = status.server || (profile ? profile.host : '') || t('top.noAddress');
  const dotState = stateTone(status.state);
  const elevation = appState.elevated
    ? ''
    : '<span class="elevation-badge">' + icon('shield') + esc(t('top.notElevated')) + '</span>';
  const langRu = UI.lang === 'ru' ? ' is-active' : '';
  const langEn = UI.lang === 'en' ? ' is-active' : '';

  host.innerHTML =
    '<button class="top-server" type="button" data-view="servers" aria-label="' + esc(t('top.openServers')) + '">' +
      '<span class="top-server-dot" data-state="' + esc(dotState) + '" aria-hidden="true"></span>' +
      '<span class="top-server-text">' +
        '<span class="top-server-name">' + esc(name) + '</span>' +
        '<span class="top-server-addr">' + esc(address) + '</span>' +
      '</span>' +
      '<span class="top-server-chevron" aria-hidden="true">' + icon('chevronDown') + '</span>' +
    '</button>' +
    '<div class="topbar-actions">' +
      elevation +
      '<div class="lang-switch" role="group" aria-label="' + esc(t('top.language')) + '">' +
        '<button class="lang-btn' + langRu + '" type="button" data-lang="ru" aria-pressed="' + (UI.lang === 'ru') + '">RU</button>' +
        '<button class="lang-btn' + langEn + '" type="button" data-lang="en" aria-pressed="' + (UI.lang === 'en') + '">EN</button>' +
      '</div>' +
    '</div>';
}

function renderView() {
  if (!['connect', 'servers', 'settings', 'logs', 'about'].includes(UI.view)) UI.view = 'connect';
  const host = $('#viewRoot');
  if (!host) return;
  const renderers = {
    connect: renderConnectView,
    servers: renderServersView,
    settings: renderSettingsView,
    logs: renderLogsView,
    about: renderAboutView
  };
  host.innerHTML = renderers[UI.view]();
  renderNav();
  if (UI.view === 'connect') {
    requestAnimationFrame(() => {
      drawChart();
      updateLiveDom();
    });
  } else if (UI.view === 'logs') {
    requestAnimationFrame(() => {
      const scroll = $('#logScroll');
      if (scroll && UI.autoScroll) scroll.scrollTop = scroll.scrollHeight;
    });
  }
}

function pageHead(title, description, actions) {
  return '<header class="page-head">' +
    '<div><h1>' + esc(title) + '</h1><p>' + esc(description) + '</p></div>' +
    (actions ? '<div class="page-head-actions">' + actions + '</div>' : '') +
  '</header>';
}

/* ---------- Connect ---------- */

function emptyConnectHtml() {
  return '<div class="view connect-view">' +
    '<section class="empty-card">' +
      '<div class="empty-icon" aria-hidden="true">' + icon('serverPlus') + '</div>' +
      '<span class="eyebrow">' + esc(t('empty.eyebrow')) + '</span>' +
      '<h1>' + esc(t('empty.title')) + '</h1>' +
      '<p>' + esc(t('empty.desc')) + '</p>' +
      '<div class="empty-actions">' +
        '<button class="btn btn-primary" type="button" data-action="open-import">' +
          icon('key') + esc(t('empty.invite')) +
        '</button>' +
        '<button class="text-link" type="button" data-action="free-server">' +
          esc(t('empty.free')) + icon('external') +
        '</button>' +
      '</div>' +
      '<div class="empty-foot">' + icon('lock') + esc(t('empty.foot')) + '</div>' +
    '</section>' +
  '</div>';
}

function wintunNoticeHtml() {
  const wintun = appState.wintun;
  if (wintun.status === 'ready') return '';
  const downloading = appState.busy === 'wintun-download';
  const isError = wintun.status === 'error';
  const title = isError ? t('notices.wintunError') : t('notices.wintunMissing');
  let text = isError ? t('notices.wintunErrorText') : t('notices.wintunMissingText');
  if (wintun.version) text += ' ' + t('notices.wintunVersion', { version: wintun.version });
  return '<div class="notice ' + (isError ? 'notice-danger' : 'notice-accent') + '">' +
    '<span class="notice-icon" aria-hidden="true">' + icon(isError ? 'alertCircle' : 'downloadCloud') + '</span>' +
    '<div class="notice-body">' +
      '<div class="notice-title">' + esc(title) + '</div>' +
      '<div class="notice-text">' + esc(text) + '</div>' +
      (downloading
        ? '<div class="notice-text" style="margin-top:4px">' + esc(t('notices.wintunDownloading')) + '</div>' +
          '<div class="progress-track" role="progressbar" aria-label="' + esc(t('notices.wintunDownloading')) + '"><span class="progress-bar"></span></div>'
        : '') +
    '</div>' +
    '<div class="notice-cta">' +
      '<button class="btn ' + (isError ? 'btn-danger' : 'btn-secondary') + ' btn-sm" type="button" data-action="install-wintun"' + (downloading ? ' disabled' : '') + '>' +
        (downloading ? '<span class="spinner"></span>' + esc(t('common.loading')) : icon('download') + esc(isError ? t('notices.retry') : t('notices.download'))) +
      '</button>' +
    '</div>' +
  '</div>';
}

function adminNoticeHtml() {
  if (appState.elevated || UI.mode !== 'tunnel') return '';
  return '<div class="notice notice-warning">' +
    '<span class="notice-icon" aria-hidden="true">' + icon('shield') + '</span>' +
    '<div class="notice-body">' +
      '<div class="notice-title">' + esc(t('notices.adminTitle')) + '</div>' +
      '<div class="notice-text">' + esc(t('notices.adminText')) + '</div>' +
    '</div>' +
    '<div class="notice-cta">' +
      '<button class="btn btn-warning btn-sm" type="button" data-action="relaunch-admin">' +
        icon('refresh') + esc(t('notices.relaunch')) +
      '</button>' +
    '</div>' +
  '</div>';
}

function connectPowerLabel(state) {
  if (UI.pending) return t('power.pending');
  if (state === 'connected') return t('power.disconnect');
  if (state === 'disconnecting') return t('power.disconnecting');
  if (state === 'connecting' || state === 'reconnecting') return t('power.cancel');
  return t('power.connect');
}

function connectCaption(state) {
  if (state === 'connected') return UI.mode === 'proxy' ? t('caption.proxy') : t('caption.connected');
  if (state === 'disconnecting') return t('caption.disconnecting');
  if (state === 'connecting' || state === 'reconnecting') return t('caption.connecting');
  if (state === 'error') return t('caption.error');
  return t('caption.disconnected');
}

function renderConnectView() {
  if (!appState.profiles.length) return emptyConnectHtml();

  const status = appState.status;
  const profile = activeProfile();
  const state = status.state;
  const serverName = status.profileName || (profile ? profile.name : '') || t('conn.fallbackServer');
  const serverAddress = status.server || (profile ? profile.host : '') || t('conn.fallbackAddress');
  const upLabel = stateLabel(state);
  const assigned = status.assignedIP || t('common.notAvailable');
  const powerLabel = connectPowerLabel(state);
  const locked = ['connected', 'connecting', 'reconnecting', 'disconnecting'].includes(state);
  const powerDisabled = UI.pending || state === 'disconnecting';
  const tunnelPressed = UI.mode === 'tunnel';
  const proxyPressed = UI.mode === 'proxy';
  const proxyAddress = '127.0.0.1:' + (appState.settings.proxyPort || 1080);

  const adminNotice = adminNoticeHtml();
  const wintunNotice = wintunNoticeHtml();
  const statusError = state === 'error' && (status.error || t('conn.error'))
    ? '<p class="status-note" title="' + esc(status.error || t('conn.error')) + '">' + esc(status.error || t('conn.error')) + '</p>'
    : '';

  return '<div class="view connect-view' + (UI.mode === 'proxy' ? ' is-proxy' : '') + '">' +
    '<section class="hero" data-state="' + esc(stateTone(state)) + '" aria-labelledby="connectionLabel">' +
      '<div class="power-col">' +
        '<div class="power-orbit">' +
          '<div class="power-halo" aria-hidden="true"></div>' +
          '<svg class="orbit" viewBox="0 0 208 208" aria-hidden="true">' +
            '<defs><linearGradient id="powerRing" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#5B8CFF"/><stop offset="1" stop-color="#8B5CF6"/></linearGradient></defs>' +
            '<circle class="orbit-track" cx="104" cy="104" r="98"/>' +
            '<circle class="orbit-progress" cx="104" cy="104" r="98"/>' +
          '</svg>' +
          '<button class="power-btn" id="powerBtn" type="button" data-action="toggle-connection"' + (powerDisabled ? ' disabled' : '') +
            ' aria-label="' + esc(powerLabel + ' · ' + serverName) + '">' +
            '<span class="power-icon" aria-hidden="true">' + icon('power') + '</span>' +
            '<span class="power-btn-label">' + esc(powerLabel) + '</span>' +
          '</button>' +
        '</div>' +
        '<div class="power-caption">' + esc(connectCaption(state)) + '</div>' +
      '</div>' +
      '<div class="status-col">' +
        '<div class="status-kicker">' +
          '<span class="status-dot" aria-hidden="true"></span>' +
          '<span class="eyebrow">' + esc(t('state.eyebrow')) + '</span>' +
        '</div>' +
        '<h1 class="conn-label" id="connectionLabel">' + esc(upLabel) + '</h1>' +
        '<div class="conn-server">' +
          '<span class="conn-server-name">' + esc(serverName) + '</span>' +
        '</div>' +
        '<div class="conn-addr">' + esc(serverAddress) + '</div>' +
        '<div class="status-meta">' +
          '<div class="meta-cell">' +
            '<span class="meta-label">' + esc(t('conn.ip')) + '</span>' +
            '<span class="meta-value" data-live="assignedIP">' + esc(assigned) + '</span>' +
          '</div>' +
          '<div class="meta-cell">' +
            '<span class="meta-label">' + esc(t('conn.uptime')) + '</span>' +
            '<span class="meta-value" data-live="uptime">' + fmtDuration(currentUptime()) + '</span>' +
          '</div>' +
        '</div>' +
        statusError +
      '</div>' +
    '</section>' +
    wintunNotice +
    adminNotice +
    renderTrafficPanel() +
    renderModePanel(locked, tunnelPressed, proxyPressed, proxyAddress) +
  '</div>';
}

function renderTrafficPanel() {
  const status = appState.status;
  const effectiveRx = status.connected ? status.rxRate : 0;
  const effectiveTx = status.connected ? status.txRate : 0;
  return '<section class="traffic-panel panel" aria-label="' + esc(t('stats.speed')) + '">' +
    '<div class="stat-grid">' +
      '<div class="stat-cell">' +
        '<span class="stat-label">' + icon('download') + esc(t('stats.downloaded')) + '</span>' +
        '<div class="stat-value"><span data-live="rxBytes">' + fmtBytes(status.rxBytes) + '</span></div>' +
        '<div class="stat-sub">' + esc(t('stats.session')) + '</div>' +
      '</div>' +
      '<div class="stat-cell">' +
        '<span class="stat-label">' + icon('upload') + esc(t('stats.uploaded')) + '</span>' +
        '<div class="stat-value"><span data-live="txBytes">' + fmtBytes(status.txBytes) + '</span></div>' +
        '<div class="stat-sub">' + esc(t('stats.session')) + '</div>' +
      '</div>' +
      '<div class="stat-cell">' +
        '<span class="stat-label">' + icon('activity') + esc(t('stats.speed')) + '</span>' +
        '<div class="stat-value"><span data-live="rxRate">' + fmtRate(effectiveRx) + '</span></div>' +
        '<div class="stat-sub"><span class="rate-tx">↑ <span data-live="txRate">' + fmtRate(effectiveTx) + '</span></span></div>' +
      '</div>' +
      '<div class="stat-cell">' +
        '<span class="stat-label">' + icon('clock') + esc(t('stats.latency')) + '</span>' +
        '<div class="stat-value"><span data-live="rtt">' + fmtLatency(status.rttMs) + '</span></div>' +
        '<div class="stat-sub">' + esc(t('stats.toServer')) + '</div>' +
      '</div>' +
    '</div>' +
    '<div class="chart-wrap">' +
      '<div class="chart-head">' +
        '<span>' + esc(t('stats.last60')) + '</span>' +
        '<div class="chart-legend">' +
          '<span class="legend-item"><span class="legend-dot"></span>' + esc(t('stats.rx')) + '</span>' +
          '<span class="legend-item"><span class="legend-dot tx"></span>' + esc(t('stats.tx')) + '</span>' +
        '</div>' +
      '</div>' +
      '<canvas id="sparkline" class="sparkline" width="640" height="80" role="img" aria-label="' + esc(t('stats.last60')) + '"></canvas>' +
    '</div>' +
  '</section>';
}

function renderModePanel(locked, tunnelPressed, proxyPressed, proxyAddress) {
  const lockTitle = locked ? ' title="' + esc(t('mode.locked')) + '"' : '';
  return '<section class="mode-panel panel">' +
    '<div class="mode-control">' +
      '<div class="segmented" role="group" aria-label="' + esc(t('mode.group')) + '"' + lockTitle + '>' +
        '<button type="button" data-action="set-mode" data-mode="tunnel" aria-pressed="' + tunnelPressed + '"' +
          (tunnelPressed ? ' class="is-active"' : '') + (locked ? ' disabled' : '') + '>' +
          icon('shield') + esc(t('mode.tunnel')) +
        '</button>' +
        '<button type="button" data-action="set-mode" data-mode="proxy" aria-pressed="' + proxyPressed + '"' +
          (proxyPressed ? ' class="is-active"' : '') + (locked ? ' disabled' : '') + '>' +
          icon('wifi') + esc(t('mode.proxy')) +
        '</button>' +
      '</div>' +
    '</div>' +
    '<div class="mode-copy">' +
      '<div class="mode-desc">' + esc(UI.mode === 'proxy' ? t('mode.proxyDesc') : t('mode.tunnelDesc')) + '</div>' +
    '</div>' +
    (UI.mode === 'proxy'
      ? '<div class="proxy-row">' +
          '<div class="proxy-address">' +
            '<span class="proxy-label">' + esc(t('mode.proxyAddress')) + '</span>' +
            '<span class="proxy-value">' +
              '<span>' + esc(proxyAddress) + '</span>' +
              '<button class="icon-btn" type="button" data-action="copy" data-copy="' + esc(proxyAddress) + '" aria-label="' + esc(t('mode.proxyAddress')) + '">' +
                icon('copy') +
              '</button>' +
            '</span>' +
          '</div>' +
          '<div class="proxy-setting">' +
            '<span>' + esc(t('mode.systemProxy')) + '</span>' +
            '<button class="switch" type="button" role="switch" data-action="toggle-system-proxy" aria-checked="' + Boolean(appState.settings.systemProxy) + '" aria-label="' + esc(t('mode.systemProxy')) + '"></button>' +
          '</div>' +
        '</div>'
      : '') +
  '</section>';
}

/* ---------- Servers ---------- */

function renderServersView() {
  const addButton = '<button class="btn btn-primary" type="button" data-action="open-add-profile">' + icon('plus') + esc(t('servers.add')) + '</button>';
  const importButton = '<button class="btn btn-secondary" type="button" data-action="open-import">' + icon('key') + esc(t('servers.import')) + '</button>';
  const header = pageHead(t('servers.title'), t('servers.desc'), addButton + importButton);

  if (!appState.profiles.length) {
    return '<div class="view">' + header +
      '<section class="panel servers-empty">' +
        '<div class="empty-icon" aria-hidden="true">' + icon('servers') + '</div>' +
        '<h2>' + esc(t('servers.emptyTitle')) + '</h2>' +
        '<p>' + esc(t('servers.emptyDesc')) + '</p>' +
        '<div class="empty-actions">' +
          '<button class="btn btn-primary" type="button" data-action="open-add-profile">' + icon('plus') + esc(t('servers.add')) + '</button>' +
          '<button class="btn btn-secondary" type="button" data-action="open-import">' + icon('key') + esc(t('servers.import')) + '</button>' +
        '</div>' +
      '</section>' +
    '</div>';
  }

  const cards = appState.profiles.map((profile) => {
    const name = profile.name || t('servers.unnamed');
    const isActive = profile.id === appState.activeProfileId;
    const isConnected = isActive && appState.status.connected;
    const isDisconnecting = appState.status.state === 'disconnecting';
    const testing = UI.testingProfileId === profile.id;
    const latency = profile.latencyMs;
    const badges = (isActive ? '<span class="badge badge-accent"><span class="badge-dot"></span>' + esc(t('servers.active')) + '</span>' : '') +
      (isConnected ? '<span class="badge badge-success"><span class="badge-dot"></span>' + esc(t('servers.connected')) + '</span>' : '');
    const connectLabel = isConnected
      ? t('servers.disconnect')
      : isDisconnecting
        ? t('power.disconnecting')
        : t('servers.connect');
    return '<article class="profile-card' + (isActive ? ' is-active' : '') + '">' +
      '<div class="profile-main">' +
        '<button class="profile-main-hit" type="button" data-action="activate-profile" data-id="' + esc(profile.id) + '" aria-label="' + esc(t('servers.activateAria', { name })) + '">' +
          '<span class="profile-icon" aria-hidden="true">' + icon('servers') + '</span>' +
          '<span class="profile-info">' +
            '<span class="profile-title-row">' +
              '<span class="profile-name">' + esc(name) + '</span>' +
              '<span class="profile-badges">' + badges + '</span>' +
            '</span>' +
            '<span class="profile-host">' + esc(profile.host || t('common.notAvailable')) + '</span>' +
            '<span class="profile-meta">' +
              '<span class="mono">' + esc(profile.tunnelAddress || t('common.notAvailable')) + '</span>' +
              '<span class="meta-sep" aria-hidden="true"></span>' +
              '<span>MTU ' + esc(profile.mtu) + '</span>' +
            '</span>' +
          '</span>' +
        '</button>' +
        '<div class="profile-side">' +
          '<span class="latency-chip ' + latencyTone(latency) + '">' + esc(fmtChipLatency(latency)) + '</span>' +
        '</div>' +
      '</div>' +
      '<div class="profile-actions">' +
        '<button class="btn ' + (isConnected ? 'btn-secondary' : 'btn-primary') + ' btn-sm" type="button" data-action="connect-profile" data-id="' + esc(profile.id) + '" aria-label="' + esc(t('servers.connectAria', { name })) + '"' + (isDisconnecting ? ' disabled' : '') + '>' +
          icon('power') + esc(connectLabel) +
        '</button>' +
        '<span class="spacer"></span>' +
        '<button class="btn btn-secondary btn-sm" type="button" data-action="test-profile" data-id="' + esc(profile.id) + '" aria-label="' + esc(t('servers.testAria', { name })) + '"' + (testing ? ' disabled' : '') + '>' +
          (testing ? '<span class="spinner"></span>' + esc(t('servers.testing')) : icon('activity') + esc(t('servers.test'))) +
        '</button>' +
        '<button class="btn btn-secondary btn-sm" type="button" data-action="edit-profile" data-id="' + esc(profile.id) + '" aria-label="' + esc(t('servers.editAria', { name })) + '">' +
          icon('edit') + esc(t('servers.edit')) +
        '</button>' +
        '<button class="btn btn-secondary btn-sm btn-delete" type="button" data-action="delete-profile" data-id="' + esc(profile.id) + '" aria-label="' + esc(t('servers.deleteAria', { name })) + '">' +
          icon('trash') + esc(t('servers.delete')) +
        '</button>' +
      '</div>' +
    '</article>';
  }).join('');

  return '<div class="view">' + header + '<div class="servers-list">' + cards + '</div></div>';
}

/* ---------- Settings ---------- */

function settingsSwitch(key, checked, disabled) {
  // The switch is a button, so it is dispatched through data-action and not
  // through the change event: the key travels in data-setting-key.
  return '<button class="switch" type="button" role="switch" data-action="toggle-setting" data-setting-key="' + esc(key) + '" aria-checked="' + Boolean(checked) + '" aria-label="' + esc(t('settings.' + key)) + '"' + (disabled ? ' disabled' : '') + '></button>';
}

function settingRow(options) {
  return '<div class="setting-row' + (options.disabled ? ' is-disabled' : '') + '">' +
    '<div class="setting-copy">' +
      '<div class="setting-title">' + esc(options.title) + '</div>' +
      (options.description ? '<div class="setting-desc">' + esc(options.description) + '</div>' : '') +
    '</div>' +
    '<div class="setting-control">' + options.control + '</div>' +
  '</div>';
}

function renderSettingsView() {
  const s = appState.settings;
  const languageControl = '<div class="segmented compact" role="group" aria-label="' + esc(t('settings.language')) + '">' +
    '<button type="button" data-lang="ru" aria-pressed="' + (UI.lang === 'ru') + '"' + (UI.lang === 'ru' ? ' class="is-active"' : '') + '>RU</button>' +
    '<button type="button" data-lang="en" aria-pressed="' + (UI.lang === 'en') + '"' + (UI.lang === 'en' ? ' class="is-active"' : '') + '>EN</button>' +
  '</div>';

  const dnsControl = '<input class="input input-md mono" id="settingDns" type="text" inputmode="decimal" spellcheck="false" autocomplete="off" data-setting="dns" value="' + esc(s.dns) + '" aria-label="' + esc(t('settings.dns')) + '" placeholder="' + esc(t('settings.dnsPlaceholder')) + '">';
  const mtuControl = '<input class="input input-sm" id="settingMtu" type="number" min="1280" max="1500" step="1" data-setting="mtu" value="' + esc(s.mtu) + '" aria-label="' + esc(t('settings.mtu')) + '">';
  const portControl = '<input class="input input-sm" id="settingProxyPort" type="number" min="1024" max="65535" step="1" data-setting="proxyPort" value="' + esc(s.proxyPort) + '" aria-label="' + esc(t('settings.proxyPort')) + '">';
  const dataDir = appState.dataDir || t('common.notAvailable');
  const dataControl = '<div class="path-box">' +
    '<span class="path-value" title="' + esc(dataDir) + '">' + esc(dataDir) + '</span>' +
    '<button class="icon-btn" type="button" data-action="copy" data-copy="' + esc(appState.dataDir) + '" aria-label="' + esc(t('settings.copyPath')) + '" title="' + esc(t('settings.copyPath')) + '">' + icon('copy') + '</button>' +
    '<button class="icon-btn" type="button" data-action="open-data-folder" aria-label="' + esc(t('settings.openData')) + '" title="' + esc(t('settings.openData')) + '">' + icon('folder') + '</button>' +
  '</div>';
  const selfTestBusy = appState.busy === 'self-test';
  const selfTestControl = '<div class="setting-control">' +
    (selfTestBusy ? '<span class="self-test-state"><span class="spinner"></span>' + esc(t('settings.selftestRunning')) + '</span>' : '') +
    '<button class="btn btn-secondary btn-sm" type="button" data-action="selftest"' + (selfTestBusy ? ' disabled' : '') + '>' +
      icon('shieldCheck') + esc(t('settings.selftest')) +
    '</button>' +
  '</div>';

  return '<div class="view">' +
    pageHead(t('settings.title'), t('settings.desc'), '') +
    '<div class="settings-layout">' +
      '<section class="settings-group panel">' +
        '<div class="settings-group-head">' + esc(t('settings.group.general')) + '</div>' +
        settingRow({ title: t('settings.language'), description: t('settings.languageDesc'), control: languageControl }) +
      '</section>' +
      '<section class="settings-group panel">' +
        '<div class="settings-group-head">' + esc(t('settings.group.network')) + '</div>' +
        settingRow({ title: t('settings.dns'), description: t('settings.dnsDesc'), control: dnsControl }) +
        settingRow({ title: t('settings.mtu'), description: t('settings.mtuDesc'), control: mtuControl }) +
        settingRow({ title: t('settings.proxyPort'), description: t('settings.proxyPortDesc'), control: portControl }) +
        settingRow({ title: t('settings.blockIPv6'), description: t('settings.blockIPv6Desc'), control: settingsSwitch('blockIPv6', s.blockIPv6, false) }) +
      '</section>' +
      '<section class="settings-group panel">' +
        '<div class="settings-group-head">' + esc(t('settings.group.startup')) + '</div>' +
        settingRow({ title: t('settings.autoStart'), description: t('settings.autoStartDesc'), control: settingsSwitch('autoStart', s.autoStart, false) }) +
        settingRow({ title: t('settings.autoConnect'), description: t('settings.autoConnectDesc'), control: settingsSwitch('autoConnect', s.autoConnect, false) }) +
        settingRow({ title: t('settings.minimizeToTray'), description: t('settings.minimizeToTrayDesc'), control: settingsSwitch('minimizeToTray', s.minimizeToTray, true), disabled: true }) +
      '</section>' +
      '<section class="settings-group panel">' +
        '<div class="settings-group-head">' + esc(t('settings.group.data')) + '</div>' +
        settingRow({ title: t('settings.dataDir'), description: t('settings.dataDirDesc'), control: dataControl }) +
        settingRow({ title: t('settings.selftest'), description: t('settings.selftestDesc'), control: selfTestControl }) +
      '</section>' +
      '<div class="settings-actions">' +
        '<button class="btn btn-secondary btn-sm" type="button" data-action="open-logs-folder">' + icon('folder') + esc(t('settings.openLogs')) + '</button>' +
        '<span class="spacer"></span>' +
        '<button class="btn btn-danger btn-sm" type="button" data-action="reset-settings">' + icon('refresh') + esc(t('settings.reset')) + '</button>' +
      '</div>' +
    '</div>' +
  '</div>';
}

/* ---------- Logs ---------- */

function logCounts() {
  const counts = { all: appState.logs.length, info: 0, warn: 0, error: 0 };
  appState.logs.forEach((entry) => {
    if (counts[entry.level] != null) counts[entry.level] += 1;
  });
  return counts;
}

function filteredLogs() {
  if (UI.logFilter === 'all') return appState.logs;
  return appState.logs.filter((entry) => entry.level === UI.logFilter);
}

function logRowsHtml() {
  const logs = filteredLogs();
  if (!logs.length) {
    return '<div class="logs-empty">' +
      '<span class="logs-empty-icon" aria-hidden="true">' + icon('terminal') + '</span>' +
      '<p>' + esc(appState.logs.length ? t('logs.emptyFiltered') : t('logs.empty')) + '</p>' +
    '</div>';
  }
  return logs.map((entry) => {
    const level = LEVELS.includes(entry.level) ? entry.level : 'info';
    const label = t('logs.level.' + level);
    return '<div class="log-row level-' + level + '">' +
      '<span class="log-time">' + esc(entry.t) + '</span>' +
      '<span class="log-level ' + level + '">' + esc(label) + '</span>' +
      '<span class="log-msg">' + esc(entry.msg) + '</span>' +
    '</div>';
  }).join('');
}

function renderLogsView() {
  const counts = logCounts();
  const filters = [
    ['all', t('logs.all'), counts.all],
    ['info', t('logs.level.info'), counts.info],
    ['warn', t('logs.level.warn'), counts.warn],
    ['error', t('logs.level.error'), counts.error]
  ];
  const filterHtml = filters.map(([level, label, count]) =>
    '<button class="filter-chip' + (UI.logFilter === level ? ' is-active' : '') + '" type="button" data-action="set-log-filter" data-level="' + level + '" aria-pressed="' + (UI.logFilter === level) + '">' +
      esc(label) +
      '<span class="filter-count" data-log-count="' + level + '">' + count + '</span>' +
    '</button>'
  ).join('');

  return '<div class="view">' +
    pageHead(t('logs.title'), t('logs.desc'), '') +
    '<section class="logs-shell">' +
      '<div class="logs-toolbar">' +
        '<div class="filter-group" role="group" aria-label="' + esc(t('logs.title')) + '">' + filterHtml + '</div>' +
        '<span class="toolbar-spacer"></span>' +
        '<div class="autoscroll-control">' +
          '<span>' + esc(t('logs.autoScroll')) + '</span>' +
          '<button class="switch" type="button" role="switch" data-action="toggle-autoscroll" aria-checked="' + Boolean(UI.autoScroll) + '" aria-label="' + esc(t('logs.autoScroll')) + '"></button>' +
        '</div>' +
        '<div class="toolbar-actions">' +
          '<button class="btn btn-ghost btn-sm" type="button" data-action="copy-logs" aria-label="' + esc(t('logs.copyAria')) + '">' + icon('copy') + esc(t('logs.copy')) + '</button>' +
          '<button class="btn btn-ghost btn-sm" type="button" data-action="clear-logs" aria-label="' + esc(t('logs.clearAria')) + '">' + icon('trash') + esc(t('logs.clear')) + '</button>' +
        '</div>' +
      '</div>' +
      '<div class="log-scroll" id="logScroll" tabindex="0" aria-label="' + esc(t('logs.title')) + '">' + logRowsHtml() + '</div>' +
    '</section>' +
  '</div>';
}

function refreshLogsView() {
  if (UI.view !== 'logs') return;
  const counts = logCounts();
  $$('[data-log-count]').forEach((element) => {
    const level = element.dataset.logCount;
    if (counts[level] != null) element.textContent = counts[level];
  });
  $$('.filter-chip[data-level]').forEach((element) => {
    const active = element.dataset.level === UI.logFilter;
    element.classList.toggle('is-active', active);
    element.setAttribute('aria-pressed', String(active));
  });
  const scroll = $('#logScroll');
  if (!scroll) return;
  const previousScroll = scroll.scrollTop;
  scroll.innerHTML = logRowsHtml();
  if (UI.autoScroll) {
    scroll.scrollTop = scroll.scrollHeight;
  } else {
    scroll.scrollTop = Math.min(previousScroll, scroll.scrollHeight);
  }
}

/* ---------- About ---------- */

function aboutLink(action, name, iconName, url) {
  return '<button class="link-row" type="button" data-action="' + action + '" data-url="' + esc(url) + '" aria-label="' + esc(t('about.openExternal', { name })) + '">' +
    '<span class="link-icon" aria-hidden="true">' + icon(iconName) + '</span>' +
    '<span>' + esc(name) + '</span>' +
    '<span class="link-arrow" aria-hidden="true">' + icon('external') + '</span>' +
  '</button>';
}

// privacyItem renders one line of the privacy statement.
function privacyItem(text) {
  return '<li class="privacy-item">' +
    '<span class="privacy-check" aria-hidden="true">' + icon('check') + '</span>' +
    '<span>' + esc(text) + '</span>' +
  '</li>';
}

function renderAboutView() {
  const version = appState.version || '2.0.0';
  return '<div class="view about-wrap">' +
    '<section class="about-hero">' +
      '<div class="about-mark" aria-hidden="true">' +
        '<svg viewBox="0 0 32 32" width="44" height="44" fill="none">' +
          '<rect x="3" y="3" width="26" height="26" rx="8" stroke="currentColor" stroke-width="2"/>' +
          '<path d="M8.4 11.6 11.35 21l4.65-7.4L20.65 21l2.95-9.4" stroke="currentColor" stroke-width="2.35" stroke-linecap="round" stroke-linejoin="round"/>' +
          '<circle cx="25.2" cy="7" r="1.35" fill="currentColor"/>' +
        '</svg>' +
      '</div>' +
      '<h1>W1nWire</h1>' +
      '<div class="about-version">' + esc(t('about.version', { version })) + '</div>' +
      '<p class="about-tagline">' + esc(t('about.tagline')) + '</p>' +
      '<div class="about-protocol">' + icon('shieldCheck') +
        '<span>' + esc(t('about.protocolLabel')) + ': ' + esc(t('about.protocol')) + '</span>' +
      '</div>' +
    '</section>' +
    '<div class="about-grid">' +
      '<section class="panel about-card">' +
        '<h2 class="about-card-title">' + esc(t('about.resources')) + '</h2>' +
        '<div class="link-list">' +
          aboutLink('open-external', t('about.github'), 'github', URLS.github) +
          aboutLink('open-external', t('about.docs'), 'book', URLS.docs) +
          aboutLink('open-external', t('about.license'), 'scale', URLS.license) +
        '</div>' +
      '</section>' +
      '<section class="panel about-card">' +
        '<h2 class="about-card-title">' + esc(t('about.privacy')) + '</h2>' +
        '<ul class="privacy-list">' +
          privacyItem(t('about.privacy.noTelemetry')) +
          privacyItem(t('about.privacy.noAccounts')) +
          privacyItem(t('about.privacy.localKeys')) +
          privacyItem(t('about.privacy.localLogs')) +
          privacyItem(t('about.privacy.openSource')) +
        '</ul>' +
      '</section>' +
      '<section class="disclaimer">' +
        '<span class="disclaimer-icon" aria-hidden="true">' + icon('alert') + '</span>' +
        '<div>' +
          '<div class="disclaimer-title">' + esc(t('about.disclaimerTitle')) + '</div>' +
          '<p>' + esc(t('about.disclaimer')) + '</p>' +
        '</div>' +
      '</section>' +
    '</div>' +
    '<div class="about-foot">' +
      '<span class="mono">W1nWire · ' + esc(version) + '</span>' +
      '<span>' + esc(t('about.build', { os: appState.os || 'windows' })) + ' · ' + esc(t('about.local')) + '</span>' +
    '</div>' +
  '</div>';
}

/* ---------- Live numbers + chart ---------- */

function updateLiveDom() {
  const status = appState.status;
  const effectiveRx = status.connected ? status.rxRate : 0;
  const effectiveTx = status.connected ? status.txRate : 0;
  setText('[data-live="rxBytes"]', fmtBytes(status.rxBytes));
  setText('[data-live="txBytes"]', fmtBytes(status.txBytes));
  setText('[data-live="rxRate"]', fmtRate(effectiveRx));
  setText('[data-live="txRate"]', fmtRate(effectiveTx));
  setText('[data-live="rtt"]', fmtLatency(status.rttMs));
  setText('[data-live="uptime"]', fmtDuration(currentUptime()));
  setText('[data-live="assignedIP"]', status.assignedIP || t('common.notAvailable'));
}

// announceState pushes a short sentence to the invisible live region, so a
// screen reader hears that the tunnel went up or down. The same text is
// announced twice in a row only if the state really changed.
function announceState(state) {
  if (!state || state === UI.lastAnnouncedState) return;
  UI.lastAnnouncedState = state;
  const region = $('#liveRegion');
  if (!region) return;
  const label = t('state.' + state);
  const name = appState.status.profileName;
  region.textContent = name ? label + ' — ' + name : label;
}

function drawChart() {
  const canvas = $('#sparkline');
  if (!canvas) return;
  const rect = canvas.getBoundingClientRect();
  const width = Math.max(1, Math.floor(rect.width || canvas.clientWidth || 460));
  const height = 80;
  const dpr = window.devicePixelRatio || 1;
  if (canvas.width !== Math.floor(width * dpr) || canvas.height !== Math.floor(height * dpr)) {
    canvas.width = Math.floor(width * dpr);
    canvas.height = Math.floor(height * dpr);
  }
  const ctx = canvas.getContext('2d');
  if (!ctx) return;
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, width, height);

  const padTop = 6;
  const padBottom = 6;
  const usable = height - padTop - padBottom;
  const rx = UI.spark.rx;
  const tx = UI.spark.tx;
  let max = 1;
  for (let index = 0; index < 60; index += 1) {
    max = Math.max(max, rx[index] || 0, tx[index] || 0);
  }

  ctx.strokeStyle = 'rgba(255,255,255,0.045)';
  ctx.lineWidth = 1;
  [0.25, 0.5, 0.75].forEach((ratio) => {
    const y = Math.round(padTop + usable * ratio) + 0.5;
    ctx.beginPath();
    ctx.moveTo(0, y);
    ctx.lineTo(width, y);
    ctx.stroke();
  });

  function seriesPoints(series) {
    return series.map((value, index) => ({
      x: (index / 59) * width,
      y: padTop + usable - (Math.max(0, value) / max) * usable
    }));
  }

  function fillSeries(points, topColor, bottomColor) {
    const gradient = ctx.createLinearGradient(0, padTop, 0, height);
    gradient.addColorStop(0, topColor);
    gradient.addColorStop(1, bottomColor);
    ctx.beginPath();
    ctx.moveTo(points[0].x, height);
    points.forEach((point) => ctx.lineTo(point.x, point.y));
    ctx.lineTo(points[points.length - 1].x, height);
    ctx.closePath();
    ctx.fillStyle = gradient;
    ctx.fill();
  }

  function strokeSeries(points, color) {
    ctx.beginPath();
    points.forEach((point, index) => {
      if (index === 0) ctx.moveTo(point.x, point.y);
      else ctx.lineTo(point.x, point.y);
    });
    ctx.strokeStyle = color;
    ctx.lineWidth = 1.6;
    ctx.lineJoin = 'round';
    ctx.lineCap = 'round';
    ctx.stroke();
  }

  const rxPoints = seriesPoints(rx);
  const txPoints = seriesPoints(tx);
  fillSeries(rxPoints, 'rgba(91,140,255,0.22)', 'rgba(91,140,255,0)');
  fillSeries(txPoints, 'rgba(61,220,151,0.10)', 'rgba(61,220,151,0)');
  strokeSeries(rxPoints, 'rgba(126,161,255,0.95)');
  strokeSeries(txPoints, 'rgba(88,214,164,0.9)');
}

/* ============================================================
   Interaction
   ============================================================ */

function bindEvents() {
  document.addEventListener('click', (event) => {
    handleClick(event).catch((error) => {
      if (!error || !error.handled) showToast('error', error && error.message ? error.message : t('toast.actionFailed'));
    });
  });
  document.addEventListener('submit', (event) => {
    handleSubmit(event).catch((error) => {
      if (!error || !error.handled) showToast('error', error && error.message ? error.message : t('toast.actionFailed'));
    });
  });
  document.addEventListener('change', handleChange);
  document.addEventListener('keydown', handleKeydown);
  window.addEventListener('beforeunload', () => {
    if (UI.eventSource) UI.eventSource.close();
  });
}

async function handleClick(event) {
  const target = event.target;
  if (!(target instanceof Element)) return;

  if (target.getAttribute('data-action') === 'modal-backdrop') {
    closeModal();
    return;
  }

  const langButton = target.closest('[data-lang]');
  if (langButton) {
    event.preventDefault();
    changeLanguage(langButton.dataset.lang === 'en' ? 'en' : 'ru', true);
    return;
  }

  const viewButton = target.closest('[data-view]');
  if (viewButton) {
    event.preventDefault();
    switchView(viewButton.dataset.view);
    return;
  }

  const actionButton = target.closest('[data-action]');
  if (!actionButton) return;
  event.preventDefault();
  try {
    await dispatchAction(actionButton.dataset.action, actionButton);
  } catch (error) {
    if (!error || !error.handled) showToast('error', error && error.message ? error.message : t('toast.actionFailed'));
  }
}

async function dispatchAction(action, element) {
  switch (action) {
    case 'toggle-connection':
      await toggleConnection();
      break;
    case 'set-mode':
      setMode(element.dataset.mode);
      break;
    case 'copy':
      await copyToClipboard(element.dataset.copy || '', element.dataset.copyMessage || undefined);
      break;
    case 'open-add-profile':
      openProfileModal('');
      break;
    case 'open-import':
      openImportModal();
      break;
    case 'edit-profile':
      openProfileModal(element.dataset.id || '');
      break;
    case 'delete-profile':
      openDeleteConfirm(element.dataset.id || '');
      break;
    case 'connect-profile':
      await connectProfile(element.dataset.id || '');
      break;
    case 'test-profile':
      await testProfile(element.dataset.id || '');
      break;
    case 'activate-profile':
      await activateProfile(element.dataset.id || '');
      break;
    case 'toggle-setting':
      toggleSetting(element);
      break;
    case 'toggle-system-proxy':
      toggleSystemProxy(element);
      break;
    case 'set-log-filter':
      UI.logFilter = ['all', 'info', 'warn', 'error'].includes(element.dataset.level) ? element.dataset.level : 'all';
      renderView();
      break;
    case 'toggle-autoscroll':
      toggleAutoScroll(element);
      break;
    case 'copy-logs':
      await copyLogs();
      break;
    case 'clear-logs':
      openClearLogsConfirm();
      break;
    case 'install-wintun':
      await installWintun();
      break;
    case 'relaunch-admin':
      await relaunchAdmin();
      break;
    case 'selftest':
      await runSelfTest();
      break;
    case 'open-logs-folder':
      await openFolder('logs');
      break;
    case 'open-data-folder':
      await openFolder('data');
      break;
    case 'open-external':
      await openExternal(element.dataset.url || '');
      break;
    case 'free-server':
      await openExternal(URLS.free);
      break;
    case 'reset-settings':
      openResetConfirm();
      break;
    case 'close-modal':
      closeModal();
      break;
    case 'modal-confirm':
      await confirmModalAction(element);
      break;
    default:
      break;
  }
}

function handleKeydown(event) {
  if (event.key === 'Escape' && UI.modal) {
    event.preventDefault();
    closeModal();
    return;
  }
  if (event.key === 'Enter' && event.target instanceof Element && event.target.matches('[data-setting]')) {
    event.preventDefault();
    event.target.blur();
    return;
  }
  if (UI.mock && (event.key === 'm' || event.key === 'M') && !event.ctrlKey && !event.metaKey && !event.altKey) {
    if (UI.modal || isEditingTarget(event.target)) return;
    event.preventDefault();
    cycleMockState();
  }
}

function switchView(view) {
  if (!['connect', 'servers', 'settings', 'logs', 'about'].includes(view)) return;
  if (UI.view === view) return;
  UI.view = view;
  const content = $('.content');
  if (content) content.scrollTop = 0;
  renderView();
  renderNav();
}

function setMode(mode) {
  if (mode !== 'tunnel' && mode !== 'proxy') return;
  if (['connected', 'connecting', 'reconnecting'].includes(appState.status.state)) return;
  UI.mode = mode;
  UI.modeTouched = true;
  renderView();
}

async function toggleConnection() {
  if (UI.pending) return;
  const state = appState.status.state;
  if (state === 'disconnecting') return;
  if (state === 'connected' || state === 'connecting' || state === 'reconnecting') {
    await disconnectConnection();
    return;
  }
  const profile = activeProfile();
  if (!profile) {
    showToast('warn', t('toast.noProfiles'));
    switchView('servers');
    return;
  }
  await connectToProfile(profile);
}

async function connectToProfile(profile) {
  if (!profile || UI.pending || appState.status.state === 'disconnecting') return;
  const previousStatus = appState.status;
  const profileId = profile.id;
  UI.pending = true;
  UI.modeTouched = true;
  appState.activeProfileId = profileId;
  appState.status = normalizeStatus({
    ...previousStatus,
    state: 'connecting',
    connected: false,
    since: null,
    uptimeSec: 0,
    profileName: profile.name,
    server: profile.host,
    mode: UI.mode,
    assignedIP: '',
    rxRate: 0,
    txRate: 0,
    rttMs: null,
    error: ''
  });
  renderAll();
  try {
    await api('/api/connect', { profileId, mode: UI.mode }, t('toast.connectError'));
  } catch (error) {
    appState.status = normalizeStatus({
      ...previousStatus,
      state: 'error',
      connected: false,
      since: null,
      uptimeSec: 0,
      profileName: profile.name,
      server: profile.host,
      mode: UI.mode,
      assignedIP: '',
      rxRate: 0,
      txRate: 0,
      rttMs: null,
      error: error.message || t('conn.error')
    });
  } finally {
    UI.pending = false;
    renderAll();
  }
}

async function disconnectConnection() {
  if (UI.pending) return;
  const previousStatus = appState.status;
  UI.pending = true;
  appState.status = normalizeStatus({
    ...previousStatus,
    state: 'disconnected',
    connected: false,
    since: null,
    uptimeSec: 0,
    assignedIP: '',
    rxRate: 0,
    txRate: 0,
    rttMs: null,
    error: ''
  });
  renderAll();
  try {
    await api('/api/disconnect', {}, t('toast.disconnectError'));
  } catch (error) {
    appState.status = previousStatus;
  } finally {
    UI.pending = false;
    renderAll();
  }
}

async function connectProfile(id) {
  if (!id || UI.pending || appState.status.state === 'disconnecting') return;
  const profile = appState.profiles.find((item) => item.id === id);
  if (!profile) {
    showToast('error', t('modal.profileNotFound'));
    return;
  }
  const currentState = appState.status.state;
  if (appState.activeProfileId === id && ['connected', 'connecting', 'reconnecting'].includes(currentState)) {
    await disconnectConnection();
    return;
  }
  if (appState.activeProfileId !== id) {
    try {
      await api('/api/profiles/activate', { id });
      appState.activeProfileId = id;
      renderView();
    } catch (error) {
      return;
    }
  }
  await connectToProfile(profile);
}

async function activateProfile(id) {
  if (!id || id === appState.activeProfileId) return;
  const profile = appState.profiles.find((item) => item.id === id);
  if (!profile) return;
  try {
    await api('/api/profiles/activate', { id });
    appState.activeProfileId = id;
    renderView();
    renderTopbar();
  } catch (error) {
    /* toast already shown */
  }
}

async function testProfile(id) {
  if (!id || UI.testingProfileId) return;
  const profile = appState.profiles.find((item) => item.id === id);
  if (!profile) return;
  UI.testingProfileId = id;
  renderView();
  try {
    const result = await api('/api/profiles/test', { id }, t('servers.testFail'));
    const rtt = result && result.rttMs != null && Number.isFinite(Number(result.rttMs)) ? Math.round(Number(result.rttMs)) : null;
    if (rtt != null) {
      profile.latencyMs = rtt;
      showToast('success', t('toast.testOk', { ms: rtt }));
    }
  } catch (error) {
    /* toast already shown */
  } finally {
    UI.testingProfileId = '';
    renderView();
  }
}

/* ---------- settings ---------- */

function toggleSetting(element) {
  const key = element.dataset.settingKey;
  if (!key || !(key in appState.settings) || element.disabled) return;
  const previous = Boolean(appState.settings[key]);
  const next = !previous;
  appState.settings[key] = next;
  element.setAttribute('aria-checked', String(next));
  const request = UI.mock ? mockApi('/api/settings', { [key]: next }) : api('/api/settings', { [key]: next });
  request.catch(() => {
    appState.settings[key] = previous;
    if (element.isConnected) element.setAttribute('aria-checked', String(previous));
  });
}

function toggleSystemProxy(element) {
  const previous = Boolean(appState.settings.systemProxy);
  const next = !previous;
  appState.settings.systemProxy = next;
  element.setAttribute('aria-checked', String(next));
  const request = UI.mock ? mockApi('/api/system-proxy', { enabled: next }) : api('/api/system-proxy', { enabled: next });
  request.catch(() => {
    appState.settings.systemProxy = previous;
    $$('[data-action="toggle-system-proxy"]').forEach((button) => {
      button.setAttribute('aria-checked', String(previous));
    });
  });
}

function handleChange(event) {
  const control = event.target.closest('[data-setting]');
  if (!control) return;
  const key = control.dataset.setting;
  if (!['dns', 'mtu', 'proxyPort'].includes(key)) return;
  const previous = appState.settings[key];
  let value;

  if (key === 'dns') {
    value = String(control.value || '').trim();
    if (!value) {
      showToast('warn', t('settings.invalidValue'));
      control.value = previous;
      control.focus();
      return;
    }
  } else {
    value = Number(control.value);
    if (!Number.isInteger(value)) {
      showToast('warn', t('settings.invalidValue'));
      control.value = previous;
      control.focus();
      return;
    }
    if (key === 'mtu' && (value < 1280 || value > 1500)) {
      showToast('warn', t('settings.invalidValue'));
      control.value = previous;
      control.focus();
      return;
    }
    if (key === 'proxyPort' && (value < 1024 || value > 65535)) {
      showToast('warn', t('settings.invalidValue'));
      control.value = previous;
      control.focus();
      return;
    }
  }

  control.removeAttribute('aria-invalid');
  if (value === previous) return;
  appState.settings[key] = value;
  const request = UI.mock ? mockApi('/api/settings', { [key]: value }) : api('/api/settings', { [key]: value });
  request.catch(() => {
    appState.settings[key] = previous;
    if (control.isConnected) control.value = previous;
  });
}

function changeLanguage(lang, persist) {
  if (lang !== 'ru' && lang !== 'en') return;
  if (lang === UI.lang) return;
  const previous = UI.lang;
  UI.lang = lang;
  appState.settings.language = lang;
  renderAll();
  if (!persist) return;
  const request = UI.mock ? mockApi('/api/settings', { language: lang }) : api('/api/settings', { language: lang }, t('toast.langError'));
  request.catch(() => {
    UI.lang = previous;
    appState.settings.language = previous;
    renderAll();
  });
}

/* ---------- logs ---------- */

function toggleAutoScroll(element) {
  UI.autoScroll = !UI.autoScroll;
  element.setAttribute('aria-checked', String(UI.autoScroll));
  if (UI.autoScroll) {
    const scroll = $('#logScroll');
    if (scroll) scroll.scrollTop = scroll.scrollHeight;
  }
}

async function copyLogs() {
  const rows = filteredLogs();
  if (!rows.length) {
    showToast('warn', t('logs.nothingToCopy'));
    return;
  }
  const text = rows.map((entry) => entry.t + ' [' + entry.level.toUpperCase() + '] ' + entry.msg).join('\n');
  await copyToClipboard(text, t('logs.copied'));
}

function openClearLogsConfirm() {
  openConfirm(
    t('logs.clearTitle'),
    t('logs.clearText'),
    t('logs.clear'),
    true,
    async () => {
      await api('/api/logs/clear', {});
      appState.logs = [];
      UI.logFilter = 'all';
      renderAll();
      showToast('success', t('logs.cleared'));
    }
  );
}

/* ---------- system actions ---------- */

async function installWintun() {
  if (appState.busy) return;
  appState.busy = 'wintun-download';
  renderView();
  renderTopbar();
  try {
    await api('/api/wintun/install', {});
    if (!UI.mock) await refreshState(true);
    showToast('success', t('toast.wintunDone'));
  } catch (error) {
    /* toast already shown */
  } finally {
    appState.busy = '';
    renderAll();
  }
}

async function relaunchAdmin() {
  try {
    await api('/api/relaunch-admin', {});
    if (UI.mock) {
      appState.elevated = true;
      renderAll();
    }
    showToast('info', t('toast.relaunch'));
  } catch (error) {
    /* toast already shown */
  }
}

async function runSelfTest() {
  if (appState.busy) return;
  appState.busy = 'self-test';
  renderView();
  renderTopbar();
  try {
    await api('/api/selftest', {}, t('toast.selftestFailed'));
    showToast('success', t('toast.selftestPassed'));
  } catch (error) {
    /* toast already shown */
  } finally {
    appState.busy = '';
    renderAll();
  }
}

async function openExternal(url) {
  if (!url) return;
  try {
    await api('/api/open-external', { url });
    showToast('success', t('toast.opened'));
  } catch (error) {
    /* toast already shown */
  }
}

async function openFolder(which) {
  try {
    await api('/api/open-folder', { which });
    showToast('success', t('toast.folderOpened'));
  } catch (error) {
    /* toast already shown */
  }
}

function openResetConfirm() {
  openConfirm(
    t('settings.resetConfirmTitle'),
    t('settings.resetConfirmText'),
    t('settings.reset'),
    true,
    async () => {
      await api('/api/settings', { ...DEFAULT_SETTINGS });
      appState.settings = normalizeSettings({ ...DEFAULT_SETTINGS });
      UI.lang = appState.settings.language;
      renderAll();
      showToast('success', t('settings.resetDone'));
    }
  );
}

/* ---------- copy ---------- */

async function copyToClipboard(text, successMessage) {
  const value = String(text == null ? '' : text);
  if (!value) {
    showToast('error', t('toast.copyFailed'));
    return false;
  }
  let copied = false;
  try {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      await navigator.clipboard.writeText(value);
      copied = true;
    }
  } catch (error) {
    copied = false;
  }
  if (!copied) {
    try {
      const area = document.createElement('textarea');
      area.value = value;
      area.setAttribute('readonly', '');
      area.style.position = 'fixed';
      area.style.left = '-9999px';
      area.style.top = '0';
      document.body.appendChild(area);
      area.select();
      copied = document.execCommand('copy');
      area.remove();
    } catch (error) {
      copied = false;
    }
  }
  showToast(copied ? 'success' : 'error', copied ? successMessage || t('toast.copied') : t('toast.copyFailed'));
  return copied;
}

/* ============================================================
   Modals
   ============================================================ */

function setModal(state) {
  if (!UI.modal) UI.lastFocus = document.activeElement;
  UI.modal = state;
  document.body.classList.add('modal-open');
  renderModal();
}

function closeModal() {
  if (!UI.modal) return;
  const restore = UI.lastFocus;
  UI.modal = null;
  UI.lastFocus = null;
  const root = $('#modalRoot');
  if (root) root.innerHTML = '';
  document.body.classList.remove('modal-open');
  if (restore && restore.isConnected) {
    requestAnimationFrame(() => {
      try {
        restore.focus({ preventScroll: true });
      } catch (error) {
        restore.focus();
      }
    });
  }
}

function openProfileModal(id) {
  let draft;
  if (id) {
    const profile = appState.profiles.find((item) => item.id === id);
    if (!profile) {
      showToast('error', t('modal.profileNotFound'));
      return;
    }
    draft = {
      name: profile.name || '',
      host: profile.host || '',
      publicKey: profile.publicKey || '',
      tunnelAddress: profile.tunnelAddress || PROFILE_DEFAULTS.tunnelAddress,
      gateway: profile.gateway || PROFILE_DEFAULTS.gateway,
      mtu: profile.mtu || PROFILE_DEFAULTS.mtu
    };
  } else {
    draft = {
      name: '',
      host: '',
      publicKey: '',
      tunnelAddress: PROFILE_DEFAULTS.tunnelAddress,
      gateway: PROFILE_DEFAULTS.gateway,
      mtu: appState.settings.mtu || PROFILE_DEFAULTS.mtu
    };
  }
  setModal({ type: 'profile', id: id || '', draft });
}

function openImportModal() {
  setModal({ type: 'import' });
}

function openClientKeyModal(key) {
  if (!key) {
    showToast('success', t('modal.serverAdded'));
    return;
  }
  setModal({ type: 'clientKey', key: String(key) });
}

function openConfirm(title, message, confirmLabel, danger, onConfirm) {
  setModal({
    type: 'confirm',
    title,
    message,
    confirmLabel,
    danger: Boolean(danger),
    onConfirm
  });
}

function openDeleteConfirm(id) {
  const profile = appState.profiles.find((item) => item.id === id);
  if (!profile) {
    showToast('error', t('modal.profileNotFound'));
    return;
  }
  const name = profile.name || t('servers.unnamed');
  openConfirm(
    t('modal.deleteTitle'),
    t('modal.deleteText', { name: esc(name) }),
    t('modal.deleteAction'),
    true,
    async () => {
      await api('/api/profiles/delete', { id });
      appState.profiles = appState.profiles.filter((item) => item.id !== id);
      if (appState.activeProfileId === id) {
        appState.activeProfileId = appState.profiles[0] ? appState.profiles[0].id : '';
      }
      renderAll();
      showToast('success', t('modal.deleted'));
    }
  );
}

function renderModal() {
  const root = $('#modalRoot');
  if (!root) return;
  if (!UI.modal) {
    root.innerHTML = '';
    return;
  }
  const state = UI.modal;
  let size = '';
  let title = '';
  let subtitle = '';
  let body = '';
  let footer = '';
  let labelledBy = 'modalTitle';

  if (state.type === 'profile') {
    const editing = Boolean(state.id);
    size = 'modal-md';
    title = editing ? t('modal.editServer') : t('modal.addServer');
    subtitle = editing ? t('modal.editServerSubtitle') : t('modal.addServerSubtitle');
    const draft = state.draft || {};
    body = '<form id="profileForm" data-form="profile" novalidate>' +
      '<div class="modal-body">' +
        '<div class="form-grid">' +
          '<div class="field">' +
            '<label for="profileName">' + esc(t('modal.name')) + '</label>' +
            '<input class="input" id="profileName" name="name" type="text" required maxlength="64" autocomplete="off" spellcheck="false" data-autofocus value="' + esc(draft.name) + '" placeholder="' + esc(t('modal.namePlaceholder')) + '">' +
          '</div>' +
          '<div class="field">' +
            '<label for="profileMtu">' + esc(t('modal.mtu')) + '</label>' +
            '<input class="input" id="profileMtu" name="mtu" type="number" min="1280" max="1500" step="1" required value="' + esc(draft.mtu) + '">' +
          '</div>' +
          '<div class="field field-full">' +
            '<label for="profileHost">' + esc(t('modal.host')) + '</label>' +
            '<input class="input mono" id="profileHost" name="host" type="text" required autocomplete="off" spellcheck="false" value="' + esc(draft.host) + '" placeholder="' + esc(t('modal.hostPlaceholder')) + '">' +
          '</div>' +
          '<div class="field field-full">' +
            '<label for="profilePublicKey">' + esc(t('modal.publicKey')) + '</label>' +
            '<input class="input mono" id="profilePublicKey" name="publicKey" type="text" required autocomplete="off" spellcheck="false" value="' + esc(draft.publicKey) + '" placeholder="' + esc(t('modal.publicKeyPlaceholder')) + '">' +
          '</div>' +
          '<div class="field">' +
            '<label for="profileTunnelAddress">' + esc(t('modal.tunnelAddress')) + '</label>' +
            '<input class="input mono" id="profileTunnelAddress" name="tunnelAddress" type="text" required autocomplete="off" spellcheck="false" value="' + esc(draft.tunnelAddress) + '">' +
          '</div>' +
          '<div class="field">' +
            '<label for="profileGateway">' + esc(t('modal.gateway')) + '</label>' +
            '<input class="input mono" id="profileGateway" name="gateway" type="text" required autocomplete="off" spellcheck="false" value="' + esc(draft.gateway) + '">' +
          '</div>' +
          '<div class="field field-full">' +
            '<div class="form-note">' + icon(editing ? 'lock' : 'key') + '<span>' + esc(editing ? t('modal.privateNoteEdit') : t('modal.privateNote')) + '</span></div>' +
          '</div>' +
        '</div>' +
      '</div>' +
      '<div class="modal-footer">' +
        '<button class="btn btn-secondary" type="button" data-action="close-modal">' + esc(t('common.cancel')) + '</button>' +
        '<button class="btn btn-primary" type="submit">' + esc(editing ? t('modal.save') : t('modal.add')) + '</button>' +
      '</div>' +
    '</form>';
  } else if (state.type === 'import') {
    size = 'modal-md';
    title = t('modal.importTitle');
    subtitle = t('modal.importSubtitle');
    body = '<form id="importForm" data-form="import" novalidate>' +
      '<div class="modal-body">' +
        '<div class="field">' +
          '<label for="importCode">' + esc(t('modal.importLabel')) + '</label>' +
          '<textarea class="textarea mono" id="importCode" name="code" required autocomplete="off" spellcheck="false" data-autofocus placeholder="' + esc(t('modal.importPlaceholder')) + '"></textarea>' +
          '<div class="invite-format">' + esc(t('empty.foot')) + '</div>' +
        '</div>' +
        '<div class="form-note" style="margin-top:16px">' + icon('lock') + '<span>' + esc(t('modal.importNote')) + '</span></div>' +
      '</div>' +
      '<div class="modal-footer">' +
        '<button class="btn btn-secondary" type="button" data-action="close-modal">' + esc(t('common.cancel')) + '</button>' +
        '<button class="btn btn-primary" type="submit">' + icon('download') + esc(t('modal.importAction')) + '</button>' +
      '</div>' +
    '</form>';
  } else if (state.type === 'clientKey') {
    title = t('modal.clientKeyTitle');
    subtitle = t('modal.clientKeyDesc');
    body = '<div class="modal-body">' +
      '<div class="field">' +
        '<span class="field-label">' + esc(t('modal.clientKeyLabel')) + '</span>' +
        '<div class="key-display">' + esc(state.key) + '</div>' +
        '<div class="key-actions">' +
          '<button class="btn btn-secondary" type="button" data-action="copy" data-copy="' + esc(state.key) + '" data-copy-message="' + esc(t('modal.keyCopied')) + '">' + icon('copy') + esc(t('modal.copyKey')) + '</button>' +
        '</div>' +
      '</div>' +
    '</div>' +
    '<div class="modal-footer">' +
      '<button class="btn btn-primary" type="button" data-action="close-modal" data-autofocus>' + esc(t('common.done')) + '</button>' +
    '</div>';
  } else if (state.type === 'confirm') {
    size = 'modal-sm';
    title = state.title;
    body = '<div class="modal-body">' +
      '<p class="confirm-copy">' + state.message + '</p>' +
    '</div>' +
    '<div class="modal-footer">' +
      '<button class="btn btn-secondary" type="button" data-action="close-modal" data-autofocus>' + esc(t('common.cancel')) + '</button>' +
      '<button class="btn ' + (state.danger ? 'btn-danger' : 'btn-primary') + '" type="button" data-action="modal-confirm">' + esc(state.confirmLabel || t('common.done')) + '</button>' +
    '</div>';
  } else {
    UI.modal = null;
    root.innerHTML = '';
    return;
  }

  root.innerHTML = '<div class="modal-backdrop" data-action="modal-backdrop">' +
    '<div class="modal ' + size + '" role="dialog" aria-modal="true" aria-labelledby="' + labelledBy + '">' +
      '<div class="modal-header">' +
        '<div>' +
          '<h2 class="modal-title" id="' + labelledBy + '">' + esc(title) + '</h2>' +
          (subtitle ? '<p class="modal-subtitle">' + esc(subtitle) + '</p>' : '') +
        '</div>' +
        '<button class="icon-btn modal-close" type="button" data-action="close-modal" aria-label="' + esc(t('common.close')) + '">' + icon('x') + '</button>' +
      '</div>' +
      body +
    '</div>' +
  '</div>';

  requestAnimationFrame(() => {
    const modal = root.querySelector('.modal');
    if (!modal) return;
    const focusable = modal.querySelector('[data-autofocus]') ||
      modal.querySelector('input:not([type="hidden"]), textarea, button');
    if (focusable) {
      try {
        focusable.focus({ preventScroll: true });
      } catch (error) {
        focusable.focus();
      }
    }
  });
}

async function confirmModalAction(element) {
  const state = UI.modal;
  if (!state || state.type !== 'confirm' || typeof state.onConfirm !== 'function') return;
  if (element) element.disabled = true;
  try {
    await state.onConfirm();
  } catch (error) {
    if (!error || !error.handled) showToast('error', error && error.message ? error.message : t('toast.actionFailed'));
  } finally {
    if (UI.modal === state) closeModal();
  }
}

/* ---------- forms ---------- */

async function handleSubmit(event) {
  const form = event.target.closest('form[data-form]');
  if (!form) return;
  event.preventDefault();
  if (form.dataset.form === 'profile') {
    await submitProfileForm(form);
  } else if (form.dataset.form === 'import') {
    await submitImportForm(form);
  }
}

function setFormPending(form, pending) {
  const submit = form.querySelector('[type="submit"]');
  if (!submit) return;
  if (pending) {
    if (!submit.dataset.originalHtml) submit.dataset.originalHtml = submit.innerHTML;
    submit.disabled = true;
    submit.innerHTML = '<span class="spinner"></span>' + esc(t('common.saving'));
  } else {
    submit.disabled = false;
    if (submit.dataset.originalHtml) {
      submit.innerHTML = submit.dataset.originalHtml;
      delete submit.dataset.originalHtml;
    }
  }
}

async function submitProfileForm(form) {
  if (!UI.modal || UI.modal.type !== 'profile') return;
  const editingId = UI.modal.id || '';
  const data = new FormData(form);
  const name = String(data.get('name') || '').trim();
  const host = String(data.get('host') || '').trim();
  const publicKey = String(data.get('publicKey') || '').trim();
  const tunnelAddress = String(data.get('tunnelAddress') || '').trim();
  const gateway = String(data.get('gateway') || '').trim();
  const mtu = Number(data.get('mtu'));

  if (!name || !host || !publicKey || !tunnelAddress || !gateway) {
    showToast('warn', t('modal.required'));
    return;
  }
  if (!Number.isInteger(mtu) || mtu < 1280 || mtu > 1500) {
    showToast('warn', t('modal.invalidMtu'));
    const mtuInput = form.querySelector('[name="mtu"]');
    if (mtuInput) mtuInput.focus();
    return;
  }

  setFormPending(form, true);
  try {
    const payload = { name, host, publicKey, tunnelAddress, gateway, mtu };
    if (editingId) {
      const result = await api('/api/profiles/update', { id: editingId, ...payload });
      const returned = result && result.profile && typeof result.profile === 'object'
        ? normalizeProfile({ ...payload, ...result.profile, id: editingId, privateKeySet: true }, 0)
        : normalizeProfile({ ...payload, id: editingId, privateKeySet: true, latencyMs: null }, 0);
      const index = appState.profiles.findIndex((profile) => profile.id === editingId);
      if (index >= 0) appState.profiles[index] = { ...appState.profiles[index], ...returned };
      closeModal();
      renderAll();
      showToast('success', t('modal.serverUpdated'));
    } else {
      const result = await api('/api/profiles/add', payload);
      const returned = result && result.profile && typeof result.profile === 'object'
        ? normalizeProfile(result.profile, appState.profiles.length)
        : normalizeProfile({
            ...payload,
            id: 'local-' + Date.now().toString(36),
            privateKeySet: true,
            createdAt: new Date().toISOString(),
            latencyMs: null
          }, appState.profiles.length);
      const exists = appState.profiles.some((profile) => profile.id === returned.id);
      if (!exists) appState.profiles.push(returned);
      if (!appState.activeProfileId) appState.activeProfileId = returned.id;
      const clientKey = result && result.publicKey ? String(result.publicKey) : '';
      closeModal();
      renderAll();
      if (clientKey) openClientKeyModal(clientKey);
      else showToast('success', t('modal.serverAdded'));
    }
  } catch (error) {
    /* API toast is enough; keep the form open */
  } finally {
    if (form.isConnected) setFormPending(form, false);
  }
}

async function submitImportForm(form) {
  const data = new FormData(form);
  const code = String(data.get('code') || '').trim();
  if (!code) {
    showToast('warn', t('modal.importEmpty'));
    return;
  }
  setFormPending(form, true);
  try {
    const result = await api('/api/profiles/import', { code });
    if (result && result.profile && typeof result.profile === 'object') {
      const profile = normalizeProfile(result.profile, appState.profiles.length);
      if (!appState.profiles.some((item) => item.id === profile.id)) appState.profiles.push(profile);
      if (!appState.activeProfileId) appState.activeProfileId = profile.id;
    }
    closeModal();
    renderAll();
    showToast('success', t('modal.imported'));
  } catch (error) {
    /* keep the form open so the code can be corrected */
  } finally {
    if (form.isConnected) setFormPending(form, false);
  }
}

/* ---------- toasts ---------- */

function showToast(kind, message) {
  const root = $('#toastRoot');
  if (!root) return;
  const safeKind = ['success', 'error', 'warn', 'info'].includes(kind) ? kind : 'info';
  const icons = {
    success: 'checkCircle',
    error: 'alertCircle',
    warn: 'alert',
    info: 'info'
  };
  while (root.children.length >= 4) root.firstElementChild.remove();
  const toast = document.createElement('div');
  toast.className = 'toast toast-' + safeKind;
  if (safeKind === 'error') toast.setAttribute('role', 'alert');
  toast.innerHTML =
    '<span class="toast-icon" aria-hidden="true">' + icon(icons[safeKind]) + '</span>' +
    '<div class="toast-body">' + esc(String(message == null ? '' : message)) + '</div>' +
    '<button class="toast-close" type="button" aria-label="' + esc(t('common.close')) + '">' + icon('x') + '</button>';
  root.appendChild(toast);

  const remove = () => {
    if (!toast.isConnected || toast.classList.contains('is-leaving')) return;
    toast.classList.add('is-leaving');
    setTimeout(() => toast.remove(), 190);
  };
  const timer = setTimeout(remove, safeKind === 'error' ? 6500 : 4400);
  toast.querySelector('.toast-close').addEventListener('click', () => {
    clearTimeout(timer);
    remove();
  });
}

/* ---------- boot ---------- */

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', init);
} else {
  init();
}


