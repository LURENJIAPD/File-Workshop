(() => {
  const buttons = document.querySelectorAll('[data-preview-page]');
  const cards = document.querySelectorAll('[data-folder]');
  const themeButton = document.querySelector('.gallery-theme-toggle');
  const schemeStorageKey = 'file-workshop-color-scheme';
  const storedScheme = localStorage.getItem(schemeStorageKey);
  let currentPage = document.querySelector('[data-preview-page].is-active')?.dataset.previewPage || 'home';
  let currentScheme = ['light', 'dark'].includes(storedScheme)
    ? storedScheme
    : (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');

  const updateThemeButton = () => {
    const isDark = currentScheme === 'dark';
    document.documentElement.dataset.colorScheme = currentScheme;
    themeButton.querySelector('span').textContent = isDark ? '☾' : '☀';
    themeButton.querySelector('strong').textContent = isDark ? '深色预览' : '浅色预览';
    themeButton.setAttribute('aria-label', `切换为${isDark ? '浅色' : '深色'}主题`);
    themeButton.setAttribute('aria-pressed', String(isDark));
  };

  const updatePreviews = () => {
    cards.forEach((card) => {
      const iframe = card.querySelector('iframe');
      const folder = card.dataset.folder;
      iframe.src = `${folder}/${currentPage}.html?embed=1&scheme=${currentScheme}`;
      iframe.title = `${card.querySelector('h3').textContent}${currentPage === 'login' ? '登录页' : '主页'}${currentScheme === 'dark' ? '深色' : '浅色'}预览`;
    });
  };

  buttons.forEach((button) => {
    button.addEventListener('click', () => {
      currentPage = button.dataset.previewPage;
      buttons.forEach((item) => item.classList.toggle('is-active', item === button));
      updatePreviews();
    });
  });

  themeButton.addEventListener('click', () => {
    currentScheme = currentScheme === 'dark' ? 'light' : 'dark';
    localStorage.setItem(schemeStorageKey, currentScheme);
    updateThemeButton();
    updatePreviews();
  });

  window.addEventListener('storage', (event) => {
    if (event.key === schemeStorageKey && ['light', 'dark'].includes(event.newValue)) {
      currentScheme = event.newValue;
      updateThemeButton();
      updatePreviews();
    }
  });

  const resizePreviews = () => {
    document.querySelectorAll('.preview-frame').forEach((frame) => {
      const scale = frame.clientWidth / 1440;
      frame.style.setProperty('--preview-scale', String(scale));
    });
  };
  window.addEventListener('resize', resizePreviews, { passive: true });
  updateThemeButton();
  updatePreviews();
  resizePreviews();
})();
