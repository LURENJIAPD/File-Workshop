(() => {
  const body = document.body;
  const page = body.dataset.page;
  const theme = body.dataset.theme || 'default';
  const params = new URLSearchParams(location.search);
  if (params.get('embed') === '1') body.classList.add('is-embedded');

  const schemeStorageKey = 'file-workshop-color-scheme';
  const storedScheme = localStorage.getItem(schemeStorageKey);
  const queryScheme = params.get('scheme');
  const systemScheme = matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  const initialScheme = ['light', 'dark'].includes(queryScheme)
    ? queryScheme
    : (['light', 'dark'].includes(storedScheme) ? storedScheme : systemScheme);
  document.documentElement.dataset.colorScheme = initialScheme;

  if (!document.querySelector('link[data-theme-modes]')) {
    const modeStyles = document.createElement('link');
    modeStyles.rel = 'stylesheet';
    modeStyles.href = new URL('modes.css', document.currentScript.src).href;
    modeStyles.dataset.themeModes = '';
    document.head.append(modeStyles);
  }

  const icons = {
    user: '<svg viewBox="0 0 24 24"><circle cx="12" cy="8" r="3.5"/><path d="M4.5 20c.8-4 3.3-6 7.5-6s6.7 2 7.5 6"/></svg>',
    lock: '<svg viewBox="0 0 24 24"><rect x="5" y="10" width="14" height="10" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/></svg>',
    eye: '<svg viewBox="0 0 24 24"><path d="M2.5 12s3.4-6 9.5-6 9.5 6 9.5 6-3.4 6-9.5 6-9.5-6-9.5-6Z"/><circle cx="12" cy="12" r="2.5"/></svg>',
    arrow: '<svg viewBox="0 0 24 24"><path d="M5 12h14m-5-5 5 5-5 5"/></svg>',
    grid: '<svg viewBox="0 0 24 24"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/></svg>',
    folder: '<svg viewBox="0 0 24 24"><path d="M3 7h7l2 2h9v10H3z"/><path d="M3 7V5h7l2 2"/></svg>',
    share: '<svg viewBox="0 0 24 24"><circle cx="18" cy="5" r="2.5"/><circle cx="6" cy="12" r="2.5"/><circle cx="18" cy="19" r="2.5"/><path d="m8.2 10.8 7.6-4.4M8.2 13.2l7.6 4.4"/></svg>',
    clock: '<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3.5 2"/></svg>',
    star: '<svg viewBox="0 0 24 24"><path d="m12 3 2.7 5.5 6.1.9-4.4 4.3 1 6.1-5.4-2.9-5.4 2.9 1-6.1-4.4-4.3 6.1-.9Z"/></svg>',
    trash: '<svg viewBox="0 0 24 24"><path d="M4 7h16M9 7V4h6v3m3 0-1 14H7L6 7m4 4v6m4-6v6"/></svg>',
    shield: '<svg viewBox="0 0 24 24"><path d="M12 3 5 6v5c0 4.7 2.7 8 7 10 4.3-2 7-5.3 7-10V6z"/><path d="m9 12 2 2 4-4"/></svg>',
    activity: '<svg viewBox="0 0 24 24"><path d="M3 12h4l2-6 4 12 2-6h6"/></svg>',
    settings: '<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="3"/><path d="M19 12a7 7 0 0 0-.1-1l2-1.5-2-3.4-2.4 1a8 8 0 0 0-1.7-1L14.5 3h-5l-.4 3.1a8 8 0 0 0-1.7 1L5 6.1 3 9.5 5.1 11a7 7 0 0 0 0 2L3 14.5l2 3.4 2.4-1a8 8 0 0 0 1.7 1l.4 3.1h5l.4-3.1a8 8 0 0 0 1.7-1l2.4 1 2-3.4-2.1-1.5a7 7 0 0 0 .1-1Z"/></svg>',
    search: '<svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/></svg>',
    bell: '<svg viewBox="0 0 24 24"><path d="M18 9a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9M10 21h4"/></svg>',
    help: '<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="9"/><path d="M9.8 9a2.4 2.4 0 1 1 3.2 2.3c-1 .4-1 1-1 1.7m0 3.5h.01"/></svg>',
    menu: '<svg viewBox="0 0 24 24"><path d="M4 7h16M4 12h16M4 17h16"/></svg>',
    chevron: '<svg viewBox="0 0 24 24"><path d="m8 10 4 4 4-4"/></svg>',
    plus: '<svg viewBox="0 0 24 24"><path d="M12 5v14M5 12h14"/></svg>',
    upload: '<svg viewBox="0 0 24 24"><path d="M12 16V4m-5 5 5-5 5 5M5 20h14"/></svg>',
    file: '<svg viewBox="0 0 24 24"><path d="M6 3h8l4 4v14H6z"/><path d="M14 3v5h5"/></svg>',
    users: '<svg viewBox="0 0 24 24"><circle cx="9" cy="8" r="3"/><path d="M3 19c.6-3.5 2.7-5.3 6-5.3s5.4 1.8 6 5.3M16 6.5a3 3 0 0 1 0 5.8m1 1.5c2.3.5 3.7 2.2 4 5.2"/></svg>',
    sparkle: '<svg viewBox="0 0 24 24"><path d="m12 3 1.2 4.1L17 9l-3.8 1.9L12 15l-1.2-4.1L7 9l3.8-1.9zM18.5 14l.8 2.2 2.2.8-2.2.8-.8 2.2-.8-2.2-2.2-.8 2.2-.8zM5.5 3l.7 1.8L8 5.5l-1.8.7L5.5 8l-.7-1.8L3 5.5l1.8-.7z"/></svg>',
    close: '<svg viewBox="0 0 24 24"><path d="m6 6 12 12M18 6 6 18"/></svg>',
    sun: '<svg class="theme-icon theme-icon-sun" viewBox="0 0 24 24"><circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M4.9 4.9l1.4 1.4m11.4 11.4 1.4 1.4M2 12h2m16 0h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>',
    moon: '<svg class="theme-icon theme-icon-moon" viewBox="0 0 24 24"><path d="M20.2 15.2A8.2 8.2 0 0 1 8.8 3.8 8.5 8.5 0 1 0 20.2 15.2Z"/></svg>'
  };

  const conceptProfiles = {
    '11-line-andon': {
      kicker: 'PLANT 03 · LINE A · ONLINE',
      title: '生产资料，<br>与产线节拍同频。',
      description: '将作业指导书、设备参数与质量文件放进同一条可追溯的信息链。',
      welcome: '华东一厂 · A 线资料中心',
      status: [['A线节拍', '42.6s', 'normal'], ['安灯状态', '正常', 'success'], ['待签工艺', '03', 'warning'], ['资料同步', '100%', 'normal']]
    },
    '12-cnc-steel': {
      kicker: 'CNC CELL · REVISION CONTROL',
      title: '每一次加工，<br>都使用正确版本。',
      description: '面向数控程序、刀具清单和设备参数的精密资料控制界面。',
      welcome: '精密加工中心 · CNC 单元',
      status: [['机台在线', '18 / 20', 'success'], ['程序校验', '通过', 'success'], ['换刀任务', '06', 'normal'], ['版本偏差', '00', 'normal']]
    },
    '13-quality-lab': {
      kicker: 'QA LAB · TRACEABLE EVIDENCE',
      title: '让质量证据，<br>比结论更清晰。',
      description: '检验规范、测量报告、8D 与批次记录在统一质量链路中闭环。',
      welcome: '质量实验室 · 检验资料台',
      status: [['今日送检', '26', 'normal'], ['待复核', '04', 'warning'], ['量具有效', '98.7%', 'success'], ['异常批次', '01', 'danger']]
    },
    '14-logistics-grid': {
      kicker: 'INTRALOGISTICS · FLOW CONTROL',
      title: '文件流转，<br>像物料一样有序。',
      description: '连接仓储、物流和生产配送资料，让每个库位与批次都有据可查。',
      welcome: '智能仓储中心 · 物流资料台',
      status: [['库位利用', '82%', 'normal'], ['AGV在线', '24 / 25', 'success'], ['待处理单据', '12', 'warning'], ['配送准时', '99.2%', 'success']]
    },
    '15-foundry-forge': {
      kicker: 'HEAVY INDUSTRY · CONTROLLED RECORDS',
      title: '重装现场，<br>也需要精确的知识秩序。',
      description: '为铸造、锻造和大型装备制造沉淀可靠、耐久、可审计的工艺档案。',
      welcome: '重装制造基地 · 工艺档案台',
      status: [['炉次记录', '08', 'normal'], ['工艺放行', '05 / 06', 'warning'], ['设备点检', '已完成', 'success'], ['高风险变更', '00', 'normal']]
    }
  };
  const profile = conceptProfiles[theme] || {
    kicker: '安全 · 协作 · 可追溯',
    title: '让每一份文件，<br>都有清晰的来处。',
    description: '统一管理企业文件、版本、共享与权限，让生产资料在正确的人之间安全流动。',
    welcome: '2026年8月4日 · 星期二'
  };

  const themeToggle = (extraClass = '') => `<button class="theme-toggle icon-button ${extraClass}" type="button" aria-label="切换为深色主题" title="切换明暗主题">${icons.sun}${icons.moon}</button>`;

  const brand = (mobile = false) => `
    <div class="brand-lockup${mobile ? ' mobile-brand' : ''}">
      <span class="brand-symbol" aria-hidden="true"></span>
      <span class="brand-name"><strong>FILE WORKSHOP</strong><small>企业文件工作台</small></span>
    </div>`;

  const loginTemplate = () => `
    <main class="login-shell">
      <section class="login-visual" aria-label="品牌介绍">
        ${brand()}
        <div class="login-visual-copy">
          <span class="concept-kicker">${profile.kicker}</span>
          <h1>${profile.title}</h1>
          <p>${profile.description}</p>
        </div>
        <div class="visual-stats" aria-label="系统状态">
          <div class="visual-stat"><strong>99.99%</strong><span>服务可用目标</span></div>
          <div class="visual-stat"><strong>256-bit</strong><span>传输与存储加密</span></div>
          <div class="visual-stat"><strong>24 / 7</strong><span>审计持续留痕</span></div>
        </div>
        <div class="visual-decoration" aria-hidden="true"><span class="deco-grid"></span><span class="deco-orbit"></span><span class="deco-file"></span></div>
      </section>
      <section class="login-panel">
        <div class="login-tools"><span class="login-concept-id">CONCEPT ${theme.replace(/\D/g, '').padStart(2, '0') || '—'}</span>${themeToggle('login-theme-toggle')}</div>
        <div class="login-card">
          <header class="login-card-head">
            ${brand(true)}
            <h2>欢迎回来</h2>
            <p>登录您的企业账号，继续处理今天的文件工作。</p>
          </header>
          <form class="login-form" novalidate>
            <label class="field"><span>账号</span><span class="input-wrap">${icons.user}<input name="username" autocomplete="username" value="wang.gong" placeholder="用户名 / 工号 / 邮箱" required></span></label>
            <label class="field"><span>密码</span><span class="input-wrap">${icons.lock}<input name="password" type="password" autocomplete="current-password" value="FileWorkshop2026" placeholder="请输入登录密码" required><button class="password-toggle" type="button" aria-label="显示密码">${icons.eye}</button></span></label>
            <div class="form-options"><label class="check-line"><input type="checkbox" checked> 在此设备保持登录</label><a href="#" data-demo="密码重置申请已发送给管理员">忘记密码？</a></div>
            <button class="primary-button" type="submit"><span>进入文件工作台</span>${icons.arrow}</button>
            <div class="login-divider">或使用企业身份</div>
            <button class="sso-button" type="button" data-demo="正在连接企业统一身份认证…">使用企业 SSO 登录</button>
          </form>
          <p class="login-help">登录遇到问题？ <a href="#" data-demo="帮助工单入口将在正式版本接入">联系系统管理员</a></p>
        </div>
      </section>
    </main>`;

  const navItems = [
    ['grid', '首页', true], ['folder', '我的文件'], ['users', '组织空间'], ['share', '共享给我的', false, '8'],
    ['clock', '最近访问'], ['star', '收藏'], ['trash', '回收站']
  ];

  const fileRows = [
    { name: 'SMT产线换线作业指导书.pdf', type: 'PDF', color: '#ef5350', space: '工艺资料库', time: '8分钟前', people: ['王', '李'] },
    { name: 'PLC程序备份_2026-08-03.zip', type: 'ZIP', color: '#8b63d9', space: '设备部', time: '34分钟前', people: ['陈'] },
    { name: '质量周报_W31.xlsx', type: 'XLS', color: '#29a56b', space: '品质中心', time: '昨天 17:42', people: ['林', '周', '+2'] },
    { name: '总装线平面布局图_V12.dwg', type: 'CAD', color: '#3179d8', space: '工程图纸', time: '昨天 14:18', people: ['赵', '王'] },
    { name: '设备点检表模板.docx', type: 'DOC', color: '#3f73cf', space: '公共资料', time: '8月2日', people: ['系'] }
  ];

  const sidebarTemplate = () => `
    <aside class="sidebar">
      ${brand()}
      <nav aria-label="主导航">
        <section class="nav-section"><span class="nav-label">工作区</span><div class="nav-list">
          ${navItems.map(([icon, label, active, count]) => `<button class="nav-item${active ? ' is-active' : ''}" type="button" data-nav="${label}">${icons[icon]}<span>${label}</span>${count ? `<span class="nav-count">${count}</span>` : ''}</button>`).join('')}
        </div></section>
        <section class="nav-section"><span class="nav-label">管理</span><div class="nav-list">
          <button class="nav-item" type="button" data-nav="空间审计">${icons.shield}<span>空间审计</span></button>
          <button class="nav-item" type="button" data-nav="系统设置">${icons.settings}<span>系统设置</span></button>
        </div></section>
      </nav>
      <div class="sidebar-bottom">
        <div class="storage-card"><div class="storage-head"><span>空间存储</span><strong>68%</strong></div><div class="progress-track"><div class="progress-value"></div></div><div class="storage-meta">6.8 TB / 10 TB</div></div>
        <a class="sidebar-user" href="login.html" title="返回登录页"><span class="avatar">王</span><span class="user-copy"><strong>王志远</strong><small>工艺工程师</small></span>${icons.chevron}</a>
      </div>
    </aside><button class="sidebar-scrim" type="button" aria-label="关闭侧栏"></button>`;

  const topbarTemplate = () => `
    <header class="topbar">
      <button class="icon-button mobile-menu" type="button" aria-label="打开菜单">${icons.menu}</button>
      <label class="search-box">${icons.search}<span class="sr-only">搜索文件</span><input type="search" placeholder="搜索文件、人员或空间…"><span class="search-shortcut">Ctrl K</span></label>
      <div class="topbar-actions">
        ${themeToggle()}
        <button class="icon-button" type="button" data-demo="帮助中心将在正式版本接入" aria-label="帮助">${icons.help}</button>
        <button class="icon-button" type="button" data-demo="您有 3 条未读通知" aria-label="通知">${icons.bell}<i class="notification-dot"></i></button>
        <div class="topbar-user">
          <button class="profile-button" type="button" aria-expanded="false"><span class="avatar">王</span><strong>王志远</strong>${icons.chevron}</button>
          <div class="profile-menu"><a href="#" data-demo="个人资料面板">个人资料</a><a href="#" data-demo="偏好设置面板">偏好设置</a><a href="login.html">退出登录</a></div>
        </div>
      </div>
    </header>`;

  const metricsTemplate = () => {
    const metrics = [
      ['folder', '我的文件', '1,284', '+32 本月', '#3d78e8'],
      ['share', '协作共享', '48', '+6 本周', '#8a63d2'],
      ['activity', '今日访问', '126', '+18.4%', '#24a56c'],
      ['shield', '安全状态', '正常', '已受保护', '#e99b28']
    ];
    return `<section class="metric-grid" aria-label="工作概览">${metrics.map(([icon, label, value, delta, color]) => `<article class="metric-card" style="--metric-color:${color}"><div class="metric-top"><span>${label}</span><span class="metric-icon">${icons[icon]}</span></div><div class="metric-value"><strong>${value}</strong><small>${delta}</small></div></article>`).join('')}</section>`;
  };

  const filesTemplate = () => `
    <section class="panel files-panel">
      <header class="panel-head"><div class="panel-title"><h2>最近文件</h2><span>${fileRows.length} 项最近活动</span></div><button class="panel-link" type="button" data-demo="正在打开全部文件…">查看全部</button></header>
      <table class="file-table"><thead><tr><th>名称</th><th>所属空间</th><th>最近修改</th><th>协作者</th><th></th></tr></thead><tbody>
        ${fileRows.map((file) => `<tr data-file-row data-name="${file.name.toLowerCase()}"><td><div class="file-name"><span class="file-icon" style="--file-color:${file.color}">${file.type}</span><span>${file.name}</span></div></td><td>${file.space}</td><td>${file.time}</td><td><span class="member-stack">${file.people.map((person, index) => `<i style="--avatar:${['#4c74c9','#cc775b','#4a9a78'][index % 3]}">${person}</i>`).join('')}</span></td><td><button class="row-menu" type="button" data-demo="文件操作菜单">•••</button></td></tr>`).join('')}
      </tbody></table><div class="empty-search">没有找到匹配的文件</div>
    </section>`;

  const quickTemplate = () => {
    const quick = [
      ['folder', '工艺资料库', '328 项', '#3d78e8'], ['users', '制造中心', '12 个空间', '#8a63d2'],
      ['share', '共享入口', '8 项待查看', '#e29a2d'], ['star', '我的收藏', '24 项', '#df5d72']
    ];
    return `<section class="panel quick-panel"><header class="panel-head"><div class="panel-title"><h2>快捷访问</h2></div><button class="panel-link" type="button" data-demo="快捷入口可在正式版本自定义">管理</button></header><div class="quick-grid">${quick.map(([icon, title, sub, color]) => `<a class="quick-card" href="#" data-demo="正在打开${title}" style="--quick-color:${color}"><span class="quick-icon">${icons[icon]}</span><span><strong>${title}</strong><small>${sub}</small></span></a>`).join('')}</div></section>`;
  };

  const activityTemplate = () => `
    <section class="panel activity-panel"><header class="panel-head"><div class="panel-title"><h2>协作动态</h2></div><button class="panel-link" type="button" data-demo="正在打开完整动态…">全部</button></header><div class="activity-list">
      <article class="activity-item" style="--activity-color:#3d78e8"><i class="activity-dot"></i><p><strong>李敏</strong> 更新了《SMT产线换线作业指导书》</p><time>8 分钟前</time></article>
      <article class="activity-item" style="--activity-color:#8a63d2"><i class="activity-dot"></i><p><strong>陈工</strong> 向您共享了 PLC 程序备份</p><time>34 分钟前</time></article>
      <article class="activity-item" style="--activity-color:#24a56c"><i class="activity-dot"></i><p><strong>系统</strong> 完成昨日文件完整性校验</p><time>2 小时前</time></article>
    </div></section>`;

  const uploadModal = () => `
    <div class="modal-backdrop" role="dialog" aria-modal="true" aria-labelledby="upload-title">
      <section class="upload-modal"><header class="modal-head"><div><h2 id="upload-title">上传文件</h2><p>文件将上传到“我的文件 / 今日工作”</p></div><button class="modal-close" type="button" aria-label="关闭">${icons.close}</button></header>
      <div class="drop-zone" tabindex="0">${icons.upload}<strong>拖放文件到这里，或点击选择</strong><span>支持大文件分片与断点续传</span></div>
      <div class="upload-progress"><div class="progress-track"><div class="progress-value"></div></div><p><span>正在安全上传演示文件…</span><strong>0%</strong></p></div></section>
    </div>`;

  const plantStatusTemplate = () => {
    if (!profile.status) return '';
    return `<section class="plant-status" aria-label="制造现场状态">${profile.status.map(([label, value, state]) => `<article class="plant-status-item is-${state}"><span>${label}</span><strong>${value}</strong><i aria-hidden="true"></i></article>`).join('')}</section>`;
  };

  const homeTemplate = () => `
    <main class="app-shell">
      ${sidebarTemplate()}
      <section class="app-main">
        ${topbarTemplate()}
        <div class="dashboard">
          <section class="welcome-row"><div class="welcome-copy"><span class="date-line">${profile.welcome}</span><h1>上午好，王工</h1><p>今天有 8 个新共享，2 项文件任务等待处理。</p></div><div class="welcome-actions"><button class="secondary-button" type="button" data-demo="新建文件夹操作">${icons.plus}新建文件夹</button><button class="upload-button js-upload" type="button">${icons.upload}上传文件</button></div></section>
          ${plantStatusTemplate()}
          ${metricsTemplate()}
          <section class="workspace-grid">${filesTemplate()}<aside class="right-stack">${quickTemplate()}${activityTemplate()}</aside></section>
          <section class="smart-strip"><div class="smart-copy"><span>DOCUMENT INTELLIGENCE</span><h2>用自然语言找到企业文件</h2><p>权限感知的智能搜索，只返回您有权访问的内容。</p></div><label class="smart-search"><span class="sr-only">智能搜索</span><input placeholder="例如：上周王工修改的最新PLC程序"><button type="button" data-demo="智能搜索演示：已找到 3 个相关文件" aria-label="提交智能搜索">${icons.sparkle}</button></label></section>
        </div>
      </section>
      ${uploadModal()}
      <div class="toast-region" aria-live="polite"></div>
    </main>`;

  document.getElementById('app').innerHTML = page === 'login' ? loginTemplate() : homeTemplate();

  const applyScheme = (scheme, persist = true) => {
    const nextScheme = scheme === 'dark' ? 'dark' : 'light';
    document.documentElement.dataset.colorScheme = nextScheme;
    if (persist) localStorage.setItem(schemeStorageKey, nextScheme);
    document.querySelectorAll('.theme-toggle').forEach((button) => {
      const target = nextScheme === 'dark' ? 'light' : 'dark';
      button.setAttribute('aria-label', `切换为${target === 'dark' ? '深色' : '浅色'}主题`);
      button.setAttribute('aria-pressed', String(nextScheme === 'dark'));
    });
  };
  applyScheme(initialScheme, false);
  document.querySelectorAll('.theme-toggle').forEach((button) => button.addEventListener('click', () => {
    applyScheme(document.documentElement.dataset.colorScheme === 'dark' ? 'light' : 'dark');
  }));
  window.addEventListener('storage', (event) => {
    if (event.key === schemeStorageKey && ['light', 'dark'].includes(event.newValue)) applyScheme(event.newValue, false);
  });

  const toastRegion = document.querySelector('.toast-region') || (() => {
    const region = document.createElement('div');
    region.className = 'toast-region';
    region.setAttribute('aria-live', 'polite');
    document.body.append(region);
    return region;
  })();
  const toast = (message) => {
    const item = document.createElement('div');
    item.className = 'toast';
    item.textContent = message;
    toastRegion.append(item);
    setTimeout(() => item.classList.add('is-leaving'), 2300);
    setTimeout(() => item.remove(), 2550);
  };

  document.querySelectorAll('[data-demo]').forEach((element) => {
    element.addEventListener('click', (event) => {
      event.preventDefault();
      toast(element.dataset.demo);
    });
  });

  if (page === 'login') {
    const form = document.querySelector('.login-form');
    const password = form.querySelector('input[type="password"]');
    const toggle = form.querySelector('.password-toggle');
    toggle.addEventListener('click', () => {
      password.type = password.type === 'password' ? 'text' : 'password';
      toggle.setAttribute('aria-label', password.type === 'password' ? '显示密码' : '隐藏密码');
    });
    form.addEventListener('submit', (event) => {
      event.preventDefault();
      const button = form.querySelector('.primary-button');
      button.classList.add('is-loading');
      button.querySelector('span').textContent = '正在验证企业身份…';
      setTimeout(() => { location.href = 'home.html'; }, 650);
    });
    return;
  }

  const profilePanel = document.querySelector('.topbar-user');
  const profileButton = profilePanel.querySelector('.profile-button');
  profileButton.addEventListener('click', () => {
    const open = profilePanel.classList.toggle('is-open');
    profileButton.setAttribute('aria-expanded', String(open));
  });
  document.addEventListener('click', (event) => {
    if (!profilePanel.contains(event.target)) {
      profilePanel.classList.remove('is-open');
      profileButton.setAttribute('aria-expanded', 'false');
    }
  });

  const search = document.querySelector('.search-box input');
  const rows = [...document.querySelectorAll('[data-file-row]')];
  const empty = document.querySelector('.empty-search');
  search.addEventListener('input', () => {
    const query = search.value.trim().toLowerCase();
    let visible = 0;
    rows.forEach((row) => {
      const match = row.dataset.name.includes(query);
      row.hidden = !match;
      if (match) visible += 1;
    });
    empty.style.display = visible ? 'none' : 'block';
  });
  document.addEventListener('keydown', (event) => {
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {
      event.preventDefault();
      search.focus();
    }
    if (event.key === 'Escape') {
      document.body.classList.remove('sidebar-open');
      document.querySelector('.modal-backdrop')?.classList.remove('is-open');
    }
  });

  document.querySelector('.mobile-menu').addEventListener('click', () => document.body.classList.add('sidebar-open'));
  document.querySelector('.sidebar-scrim').addEventListener('click', () => document.body.classList.remove('sidebar-open'));
  document.querySelectorAll('[data-nav]').forEach((item) => item.addEventListener('click', () => {
    document.querySelectorAll('[data-nav]').forEach((nav) => nav.classList.toggle('is-active', nav === item));
    document.body.classList.remove('sidebar-open');
    toast(`已切换到“${item.dataset.nav}”演示视图`);
  }));

  const modal = document.querySelector('.modal-backdrop');
  const progress = modal.querySelector('.upload-progress');
  const progressBar = progress.querySelector('.progress-value');
  const progressText = progress.querySelector('strong');
  const openModal = () => modal.classList.add('is-open');
  const closeModal = () => modal.classList.remove('is-open');
  document.querySelector('.js-upload').addEventListener('click', openModal);
  modal.querySelector('.modal-close').addEventListener('click', closeModal);
  modal.addEventListener('click', (event) => { if (event.target === modal) closeModal(); });
  modal.querySelector('.drop-zone').addEventListener('click', () => {
    progress.classList.add('is-active');
    let value = 0;
    const timer = setInterval(() => {
      value = Math.min(100, value + 8 + Math.round(Math.random() * 10));
      progressBar.style.width = `${value}%`;
      progressText.textContent = `${value}%`;
      if (value === 100) {
        clearInterval(timer);
        setTimeout(() => { closeModal(); toast('演示文件上传完成，已进入安全扫描'); }, 450);
      }
    }, 180);
  });
})();
