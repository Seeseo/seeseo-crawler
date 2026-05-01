<script>
  import { t } from '../i18n/index.svelte.js';
  import { fmtN, fmtSize } from '../utils.js';
  import { getProjectsPaginated } from '../api.js';

  let {
    theme,
    darkMode,
    sessions,
    projects,
    globalStats,
    systemStats,
    selectedSession,
    selectedProject,
    currentView,
    liveProgress,
    ontoggledarkmode,
    onselectsession,
    onselectproject,
    onnavigate,
    onopensettings,
    onopenstats,
    onopenapi,
    onopenlogs,
    ongohome,
    oncreateproject,
    onviewallprojects,
    appVersion,
  } = $props();

  let isDark = $derived(
    darkMode === 'auto' ? window.matchMedia('(prefers-color-scheme: dark)').matches : !!darkMode,
  );

  /** @param {HTMLElement} node */
  function focusOnMount(node) {
    node.focus();
  }

  let creatingProject = $state(false);
  let newProjectName = $state('');

  // Search state
  let projectSearch = $state('');
  let searchResults = $state(null);
  let searchTimer = null;

  function startCreate() {
    creatingProject = true;
    newProjectName = '';
  }

  function confirmCreate() {
    const name = newProjectName.trim();
    if (name) {
      oncreateproject?.(name);
    }
    creatingProject = false;
    newProjectName = '';
  }

  function cancelCreate() {
    creatingProject = false;
    newProjectName = '';
  }

  function onSearchInput(e) {
    const val = e.target.value;
    projectSearch = val;
    if (searchTimer) clearTimeout(searchTimer);
    if (!val.trim()) {
      searchResults = null;
      return;
    }
    searchTimer = setTimeout(async () => {
      try {
        const res = await getProjectsPaginated(30, 0, val.trim());
        searchResults = res.projects;
      } catch {
        searchResults = null;
      }
    }, 300);
  }

  let displayedProjects = $derived(searchResults !== null ? searchResults : projects.slice(0, 30));

  let collapsed = $state(localStorage.getItem('sidebar-collapsed') === 'true');

  function applySidebarWidth() {
    document.documentElement.style.setProperty('--sidebar-width', collapsed ? '56px' : '260px');
  }
  applySidebarWidth();

  function toggleCollapse() {
    collapsed = !collapsed;
    localStorage.setItem('sidebar-collapsed', collapsed);
    applySidebarWidth();
  }
</script>

<aside class="sidebar" class:collapsed>
  <div class="sidebar-header">
    {#if theme.logo_url}
      <img class="sidebar-logo" src={theme.logo_url} alt="Logo" />
    {:else}
      <svg
        class="sidebar-logo"
        viewBox="-10 -8 170 215"
        width="36"
        height="36"
        xmlns="http://www.w3.org/2000/svg"
      >
        <g fill={isDark ? '#ffffff' : '#0a0a0a'}>
          <path d="M144.888 48.0594C144.858 47.9072 144.858 47.7854 144.995 47.6789C144.629 46.37 144.232 45.0458 143.851 43.7369C143.348 42.7476 142.829 41.7583 142.311 40.769C139.917 36.0813 136.135 32.8242 131.407 30.7086C127.549 28.9887 123.416 28.1669 119.238 27.8929C115.73 27.6646 112.161 27.2537 108.822 28.9735C108.181 29.2931 107.876 29.7802 107.724 30.3129C107.602 30.7847 108.135 31.0282 108.517 31.1956C110.957 32.3067 113.503 32.9612 116.203 32.9764C119.298 32.9764 122.364 33.2503 125.399 33.9961C128.235 34.7115 130.858 35.8225 133.161 37.6489C136.546 40.3276 137.873 44.0565 138.224 48.1659C138.498 51.3926 137.98 54.4214 135.875 57.0544C132.688 61.0877 128.891 64.497 124.773 67.5258C124.209 67.9367 123.645 68.3477 123.065 68.7434C121.678 69.7327 120.259 70.6611 118.826 71.5287C118.734 71.5895 118.643 71.6352 118.551 71.6961C114.83 73.9791 110.957 75.9729 106.946 77.6775C105.467 78.3168 103.957 78.9256 102.432 79.4887C100.907 80.0823 99.3667 80.6302 97.7959 81.1325C96.3014 81.6195 94.7917 82.0913 93.2819 82.5479C88.7222 83.9482 84.0861 85.181 79.3739 86.0942C77.4371 86.4595 75.5004 86.8704 73.5484 87.2357C72.3741 87.4792 71.1846 87.6923 69.9951 87.8597C69.0649 87.9967 68.1346 88.1489 67.2043 88.3011C67.1281 88.3011 67.0518 88.3163 66.9756 88.3315C64.6271 88.7273 62.2938 89.1078 59.9301 89.3665C56.3311 89.7774 52.7016 90.1123 49.0873 90.3862C49.011 90.3862 48.95 90.4015 48.8738 90.4015C48.4925 90.4471 48.1113 90.4776 47.73 90.508H47.6233C38.9765 91.1168 30.3145 91.5277 21.6677 90.2493C17.3977 89.6405 13.2192 88.5903 9.52868 86.1855C8.46118 85.4854 7.37842 84.6787 7.16492 83.2937C6.95142 81.9696 7.07342 80.6302 7.71393 79.4278C10.3522 74.4357 15.0492 71.7874 19.7462 69.1696C20.2037 68.9108 20.402 69.1543 20.6917 69.5044C23.5587 73.005 27.3255 75.1358 31.687 76.1555C38.8698 77.8602 46.144 78.0885 53.4488 77.2818C53.4793 77.2818 53.4946 77.2818 53.5251 77.2666C53.9826 77.2209 54.4706 77.1601 54.9433 77.0992C54.9891 77.0992 55.0501 77.084 55.0958 77.084C59.5793 76.536 64.0476 75.8055 68.4701 74.9532C71.4744 74.3596 74.4634 73.6595 77.4219 72.8224C80.4414 71.97 83.4761 71.1938 86.4347 70.0675C88.3257 69.3522 90.2777 68.6825 92.1534 67.8606C94.8679 66.6735 97.5672 65.4407 100.205 64.0404C100.205 64.0404 100.236 64.0404 100.251 64.0252C100.663 63.7969 101.075 63.5686 101.486 63.3555C103.179 62.4119 104.826 61.4226 106.397 60.3115C108.303 58.9874 110.103 57.4958 111.719 55.7607C114.937 52.2906 116.889 46.9636 113.885 41.7583C112.71 39.7493 111.277 38.0142 109.111 37.0401C106.565 35.8986 104.094 36.5835 101.776 37.6641C98.8787 39.0035 96.2099 40.7538 93.4954 42.428C90.3692 44.3153 87.2277 46.1569 83.9184 47.755C81.2649 49.0487 78.5199 50.1902 75.7291 51.1643C71.5049 52.6406 67.1586 53.7213 62.7361 54.4518C60.3876 54.8475 58.0238 55.1367 55.6448 55.4259C52.2898 55.852 48.9348 56.2173 45.5798 56.6283C42.4535 57.024 39.3425 57.3893 36.262 57.8915C33.8068 58.272 31.382 58.7895 28.9725 59.3374C27.6305 59.6266 26.2732 59.931 25.038 60.5398C23.8027 61.1334 23.7722 61.1791 23.452 59.9006C22.598 56.6283 21.8812 53.356 21.2102 50.038C20.7375 47.6485 20.1885 45.2741 19.7005 42.8846C18.9837 39.384 18.1755 35.9138 17.7027 32.3524C17.352 29.7041 17.0165 27.071 17.2757 24.4075C17.6112 21.3026 18.5415 18.3652 21.195 16.4322C23.3605 14.8646 25.7395 13.7079 28.4997 13.3121C31.5955 12.886 34.676 12.7794 37.7413 13.4187C42.2248 14.3623 46.6473 15.5799 51.0698 16.7671C58.1153 18.6848 65.2218 20.1003 72.5114 20.4503C78.0776 20.7091 83.5219 20.085 88.5544 17.4672C92.1229 15.6104 94.8222 12.2924 92.6567 7.61986C90.9944 4.02794 88.2342 1.59274 84.4064 0.679539C78.4436 -0.735921 72.4809 0.359919 66.5486 1.2579C64.8253 1.53186 63.0868 1.85148 61.3788 2.26242C59.0456 2.79512 56.6056 2.94732 54.4401 4.0736C53.8301 4.378 53.0371 4.71284 53.1896 5.4434C53.3268 6.15874 54.2113 6.00654 54.7603 5.99132C57.3071 5.9 59.8691 5.80868 62.4158 5.5956C67.6924 5.139 72.9689 4.65196 78.2606 4.6824C80.8684 4.6824 83.5066 4.80416 85.7637 6.41748C87.1057 7.37634 88.2037 8.50262 87.9902 10.2834C87.8377 11.6684 86.5719 12.125 85.5959 12.6577C83.2169 13.9514 80.5786 14.3928 77.9251 14.5906C71.3524 15.1081 64.8711 14.2101 58.4661 12.8251C52.6863 11.5618 46.9523 10.1312 41.1573 8.95922C34.6455 7.63508 28.3167 7.81772 22.11 10.4356C16.62 12.7338 13.5699 16.9041 12.4109 22.5202C11.1604 28.6691 12.4109 34.7723 13.1734 40.8603C13.5242 43.5847 13.9969 46.2787 14.3477 49.0031C14.7594 52.1536 15.2169 55.3041 15.9794 58.3786C16.3455 59.8854 16.5895 61.4226 17.2605 62.8228C17.4892 63.2946 17.535 63.6295 16.864 63.9795C13.7377 65.5624 10.7182 67.2975 7.94268 69.4283C6.00592 70.9046 4.16067 72.5027 2.87966 74.5879C1.14116 77.4188 -0.353348 80.3867 0.0736537 83.8873C0.180404 84.2374 0.271904 84.5722 0.378655 84.9375C1.12591 86.1246 1.65966 87.4336 2.52891 88.5751C4.37417 90.9494 6.70742 92.7301 9.40668 93.9173C17.5045 97.4635 26.1207 98.2093 34.8438 98.4833C38.763 98.605 42.6975 98.5289 46.6015 98.3159C47.2115 98.2854 47.8215 98.255 48.4315 98.2245C48.4773 98.2245 48.523 98.2245 48.5688 98.2093C49.6668 98.1637 50.7648 98.0876 51.8628 98.0115C56.2091 97.6918 60.5553 97.2352 64.8863 96.5808C64.9321 96.5808 64.9778 96.5656 65.0236 96.5656C65.3591 96.5199 65.6946 96.459 66.0301 96.4134C66.8078 96.2916 67.5703 96.1698 68.3481 96.0481C71.0779 95.6219 73.7924 95.1501 76.4916 94.587C79.5721 93.9782 82.6221 93.278 85.6264 92.3953C85.9619 92.2887 86.3127 92.1213 86.6787 92.304C88.7679 91.6952 90.8419 91.1016 92.9312 90.4928C93.0837 90.2036 93.3734 90.1275 93.6479 90.0666C96.1489 89.473 98.5584 88.6055 100.968 87.6923C101.974 87.3118 102.981 86.9161 103.987 86.5051C105.329 85.9572 106.671 85.3941 108.013 84.8309C108.349 84.694 108.669 84.557 109.005 84.4048C110.316 83.8569 111.612 83.2785 112.893 82.6545C114.662 81.8326 116.401 80.9498 118.124 80.0366C120.015 79.0169 121.891 77.9667 123.706 76.8252C123.736 76.8252 123.767 76.7948 123.797 76.7796C125.216 75.9272 126.603 75.0293 127.961 74.1008C130.233 72.5332 132.414 70.859 134.503 69.0326C137.263 66.5974 139.856 64.01 141.93 60.966C144.553 57.0697 145.94 52.8233 144.888 48.0594ZM27.3255 65.882C27.661 65.7451 28.0117 65.669 28.3777 65.5624C31.5497 64.908 34.7523 64.2992 37.9853 63.8426C43.765 63.0207 49.5905 62.8076 55.3856 62.0618C59.7013 61.5139 64.0171 60.966 68.2719 59.9767C70.5289 59.444 72.8011 58.9265 74.9666 58.1655C80.7464 56.1717 86.2669 53.6147 91.4977 50.4033C95.6609 47.8615 99.3972 44.7262 103.56 42.2149C104.201 41.8192 104.841 41.4387 105.573 41.2256C107.266 40.7386 108.593 41.3474 109.264 43.0216C110.56 46.2482 110.103 49.4596 107.312 52.0623C104.323 54.8323 100.922 56.9175 97.3232 58.7591C97.2164 58.82 97.0944 58.8656 96.9877 58.9265C94.5324 60.1745 92.0314 61.2856 89.4999 62.3358C88.8594 62.5945 88.2189 62.8533 87.5784 63.112C87.3802 63.2033 87.1972 63.2642 86.9989 63.3555C82.8204 64.9841 78.5199 66.2017 74.1431 67.2366C71.6116 67.815 69.0344 68.3477 66.4571 68.7738C66.1063 68.8347 65.7556 68.8804 65.4201 68.9413C64.9168 69.0174 64.3983 69.0935 63.8951 69.1696C62.5683 69.3674 61.2416 69.5501 59.9148 69.7327C58.6338 69.8849 57.3681 70.0371 56.0871 70.1741C51.2528 70.6763 46.4033 71.0416 41.5385 70.7829C37.7108 70.5698 33.9135 70.0828 30.3145 68.6369C29.2012 68.1955 28.1337 67.5867 27.1425 66.8409C26.4105 66.2778 26.7307 66.0799 27.3255 65.882Z" />
          <path d="M96.9877 58.9265C94.5324 60.1746 92.0314 61.2856 89.4999 62.3358C88.8594 62.5945 88.2189 62.8533 87.5784 63.112C87.3801 63.2033 87.1971 63.2642 86.9989 63.3555C82.8204 64.9841 78.5199 66.2017 74.1431 67.2366C71.6116 67.815 69.0343 68.3477 66.4571 68.7739C66.1063 68.8347 65.7556 68.8804 65.4201 68.9413C64.9168 69.0174 64.3983 69.0935 63.8951 69.1696C62.5683 69.3674 61.2416 69.5501 59.9148 69.7327C57.6578 71.7113 55.6905 73.9487 54.0435 76.46C53.8758 76.7187 53.6928 76.9927 53.525 77.2666C53.9825 77.221 54.4705 77.1601 54.9433 77.0992C54.989 77.0992 55.05 77.084 55.0958 77.084C59.5793 76.5361 64.0476 75.8055 68.4701 74.9532C71.4743 74.3596 74.4633 73.6595 77.4219 72.8224C80.4414 71.9701 83.4761 71.1938 86.4346 70.0676C88.3256 69.3522 90.2776 68.6825 92.1534 67.8607C94.8679 66.6735 97.5672 65.4407 100.205 64.0404C100.205 64.0404 100.236 64.0404 100.251 64.0252C100.663 63.7969 101.075 63.5686 101.486 63.3555C103.179 62.4119 104.826 61.4226 106.397 60.3115C103.454 59.5962 100.312 59.1244 96.9877 58.9265ZM128.997 75.5772C128.662 75.0749 128.327 74.5727 127.961 74.1009C126.527 72.1375 124.895 70.3415 123.065 68.7434C121.677 69.7327 120.259 70.6611 118.826 71.5287C118.734 71.5896 118.643 71.6352 118.551 71.6961C114.83 73.9791 110.957 75.9729 106.946 77.6776C108.09 78.3016 109.157 79.0321 110.148 79.854C111.17 80.7063 112.085 81.6348 112.893 82.6545C115.471 85.8811 116.935 89.9449 117.301 94.8305H134.976C134.503 87.3423 132.505 80.9194 128.997 75.5772ZM129.18 132.013C123.873 126.245 115.928 122.166 105.329 119.791L84.0556 114.799C77.9861 113.368 73.4568 111.085 70.4831 107.935C67.7076 104.997 66.2283 101.162 66.0301 96.4134C66.0148 96.0633 66.0148 95.7133 66.0148 95.3632C66.0148 92.791 66.4113 90.4319 67.2043 88.3011C67.1281 88.3011 67.0518 88.3163 66.9756 88.3316C64.6271 88.7273 62.2938 89.1078 59.93 89.3665C56.331 89.7775 52.7015 90.1123 49.0873 90.3863C49.011 90.3863 48.95 90.4015 48.8738 90.4015C48.6298 92.167 48.5078 94.0086 48.5078 95.8959C48.5078 96.6873 48.523 97.4483 48.5688 98.2093C48.9805 106.443 51.6798 113.308 56.636 118.817C62.0498 124.814 70.0103 128.999 80.4871 131.374L101.746 136.198C107.937 137.629 112.466 139.943 115.333 143.154C118.185 146.365 119.619 150.52 119.619 155.634C119.619 161.935 117.087 166.989 112.024 170.794C106.961 174.599 100.144 176.501 91.5739 176.501C83.4609 176.501 76.9796 174.69 72.0996 171.052C67.2043 167.43 64.7033 162.529 64.5813 156.35H46.7235C47.1963 163.594 49.316 169.926 53.0675 175.329C56.819 180.747 62.0041 184.902 68.6073 187.81C75.2258 190.732 82.8661 192.193 91.5739 192.193C100.739 192.193 108.745 190.64 115.593 187.551C122.44 184.461 127.747 180.063 131.499 174.355C135.25 168.648 137.126 161.935 137.126 154.204C137.126 145.178 134.472 137.766 129.18 132.013Z" />
        </g>
        <circle cx="155" cy="195" r="10" fill="#FF6B00" />
      </svg>
    {/if}
    {#if !collapsed}<span class="sidebar-app-name">{theme.app_name}</span>{/if}
  </div>

  <div class="sidebar-section">
    {#if !collapsed}<div class="sidebar-section-title">{t('sidebar.mainMenu')}</div>{/if}
    <nav class="sidebar-nav">
      <button
        class="sidebar-link"
        class:active={currentView === 'home'}
        onclick={() => ongohome?.()}
        title={collapsed ? t('sidebar.dashboard') : undefined}
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          ><rect x="3" y="3" width="7" height="7" /><rect x="14" y="3" width="7" height="7" /><rect
            x="3"
            y="14"
            width="7"
            height="7"
          /><rect x="14" y="14" width="7" height="7" /></svg
        >
        {#if !collapsed}{t('sidebar.dashboard')}{/if}
      </button>
      <button
        class="sidebar-link"
        class:active={currentView === 'new-crawl'}
        onclick={() => onnavigate?.('/new-crawl')}
        title={collapsed ? t('sidebar.newCrawl') : undefined}
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          ><circle cx="12" cy="12" r="10" /><line x1="12" y1="8" x2="12" y2="16" /><line
            x1="8"
            y1="12"
            x2="16"
            y2="12"
          /></svg
        >
        {#if !collapsed}{t('sidebar.newCrawl')}{/if}
      </button>
      <button
        class="sidebar-link"
        class:active={currentView === 'compare'}
        onclick={() => onnavigate?.('/compare')}
        title={collapsed ? t('sidebar.compare') : undefined}
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          ><line x1="18" y1="20" x2="18" y2="10" /><line x1="12" y1="20" x2="12" y2="4" /><line
            x1="6"
            y1="20"
            x2="6"
            y2="14"
          /></svg
        >
        {#if !collapsed}{t('sidebar.compare')}{/if}
      </button>
    </nav>
  </div>

  {#if !collapsed}
    <details class="sidebar-details" open>
      <summary class="sidebar-section-title flex-between">
        <span>{t('sidebar.projects')}</span>
        <button
          class="sidebar-add-btn"
          onclick={(e) => {
            e.stopPropagation();
            startCreate();
          }}
          title={t('sidebar.newProject')}
        >
          <svg
            viewBox="0 0 24 24"
            width="14"
            height="14"
            fill="none"
            stroke="currentColor"
            stroke-width="2.5"
            stroke-linecap="round"
            stroke-linejoin="round"
            ><line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" /></svg
          >
        </button>
      </summary>
      <div class="sidebar-details-body">
        <div class="sidebar-search">
          <input
            class="sidebar-search-input"
            type="text"
            placeholder={t('sidebar.searchProjects')}
            value={projectSearch}
            oninput={onSearchInput}
          />
        </div>
        {#if creatingProject}
          <div class="sidebar-inline-input">
            <input
              type="text"
              bind:value={newProjectName}
              placeholder={t('sidebar.projectName')}
              use:focusOnMount
              onkeydown={(e) => {
                if (e.key === 'Enter') confirmCreate();
                if (e.key === 'Escape') cancelCreate();
              }}
              onblur={cancelCreate}
            />
          </div>
        {/if}
        <nav class="sidebar-nav">
          {#each displayedProjects as proj}
            {@const projStats = globalStats?.projects?.find((p) => p.project_id === proj.id)}
            <div class="sidebar-project">
              <button
                class="sidebar-link sidebar-project-header"
                class:active={selectedProject?.id === proj.id}
                onclick={() => onselectproject?.(proj)}
              >
                <svg
                  viewBox="0 0 24 24"
                  width="15"
                  height="15"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  ><path
                    d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"
                  /></svg
                >
                <span class="truncate sidebar-project-name">{proj.name}</span>
                {#if projStats}
                  <span class="sidebar-badge">{fmtN(projStats.total_pages)}</span>
                {/if}
              </button>
            </div>
          {/each}
        </nav>
        {#if projects.length > 30 || searchResults !== null}
          <button
            class="sidebar-link sidebar-view-all"
            class:active={currentView === 'all-projects'}
            onclick={() => onviewallprojects?.()}
          >
            {t('sidebar.viewAllProjects')} &rarr;
          </button>
        {/if}
      </div>
    </details>

    {#if sessions.filter((s) => !s.ProjectID).length > 0}
      <div class="sidebar-section">
        <div class="sidebar-section-title">{t('sidebar.unassigned')}</div>
        <nav class="sidebar-nav">
          {#each sessions.filter((s) => !s.ProjectID).slice(0, 5) as s}
            <button
              class="sidebar-link"
              class:active={selectedSession?.ID === s.ID}
              onclick={() => onselectsession?.(s)}
            >
              <svg
                viewBox="0 0 24 24"
                width="14"
                height="14"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
                ><path
                  d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"
                /></svg
              >
              <span class="truncate">
                {#if s.is_running}
                  <span class="text-info"
                    >{new URL(s.SeedURLs?.[0] || 'https://unknown').hostname}</span
                  >
                {:else}
                  {new URL(s.SeedURLs?.[0] || 'https://unknown').hostname}
                {/if}
              </span>
            </button>
          {/each}
        </nav>
      </div>
    {/if}

    {#if systemStats}
      <details class="sidebar-details">
        <summary class="sidebar-section-title">{t('sidebar.system')}</summary>
        <div class="sidebar-details-body">
          <div class="sidebar-stats">
            <div class="sidebar-stat">
              <span class="sidebar-stat-label">{t('sidebar.memory')}</span>
              <span class="sidebar-stat-value">{fmtSize(systemStats.mem_alloc)}</span>
            </div>
            <div class="sidebar-stat">
              <span class="sidebar-stat-label">{t('sidebar.heap')}</span>
              <span class="sidebar-stat-value">{fmtSize(systemStats.mem_heap_inuse)}</span>
            </div>
            <div class="sidebar-stat">
              <span class="sidebar-stat-label">{t('sidebar.sys')}</span>
              <span class="sidebar-stat-value">{fmtSize(systemStats.mem_sys)}</span>
            </div>
            <div class="sidebar-stat">
              <span class="sidebar-stat-label">{t('sidebar.goroutines')}</span>
              <span class="sidebar-stat-value">{fmtN(systemStats.num_goroutines)}</span>
            </div>
            <div class="sidebar-stat">
              <span class="sidebar-stat-label">{t('sidebar.gcCycles')}</span>
              <span class="sidebar-stat-value">{fmtN(systemStats.num_gc)}</span>
            </div>
          </div>
        </div>
      </details>
    {/if}
  {/if}

  <div class="sidebar-section sidebar-section-push">
    {#if !collapsed}<div class="sidebar-section-title">{t('sidebar.general')}</div>{/if}
    <nav class="sidebar-nav">
      <button
        class="sidebar-link"
        class:active={currentView === 'stats'}
        onclick={() => onopenstats?.()}
        title={collapsed ? t('sidebar.stats') : undefined}
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          ><line x1="18" y1="20" x2="18" y2="10" /><line x1="12" y1="20" x2="12" y2="4" /><line
            x1="6"
            y1="20"
            x2="6"
            y2="14"
          /></svg
        >
        {#if !collapsed}{t('sidebar.stats')}{/if}
      </button>
      <button
        class="sidebar-link"
        class:active={currentView === 'settings'}
        onclick={() => onopensettings?.()}
        title={collapsed ? t('sidebar.settings') : undefined}
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          ><circle cx="12" cy="12" r="3" /><path
            d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"
          /></svg
        >
        {#if !collapsed}{t('sidebar.settings')}{/if}
      </button>
      <button
        class="sidebar-link"
        class:active={currentView === 'logs'}
        onclick={() => onopenlogs?.()}
        title={collapsed ? t('sidebar.logs') : undefined}
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          ><polyline points="4 17 10 11 4 5" /><line x1="12" y1="19" x2="20" y2="19" /></svg
        >
        {#if !collapsed}{t('sidebar.logs')}{/if}
      </button>
      <button
        class="sidebar-link"
        class:active={currentView === 'api'}
        onclick={() => onopenapi?.()}
        title={collapsed ? t('sidebar.api') : undefined}
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          ><path
            d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"
          /></svg
        >
        {#if !collapsed}{t('sidebar.api')}{/if}
      </button>
    </nav>
  </div>

  <div class="sidebar-footer">
    <div class="sidebar-footer-actions">
      <button
        class="sidebar-icon-btn"
        onclick={() => ontoggledarkmode?.()}
        title={darkMode === 'auto'
          ? t('settings.auto')
          : darkMode
            ? t('sidebar.lightMode')
            : t('sidebar.darkMode')}
      >
        {#if darkMode === 'auto'}
          <svg
            viewBox="0 0 24 24"
            width="16"
            height="16"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            ><circle cx="12" cy="12" r="9" /><path
              d="M12 3a9 9 0 0 1 0 18z"
              fill="currentColor"
            /></svg
          >
        {:else if darkMode}
          <svg
            viewBox="0 0 24 24"
            width="16"
            height="16"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            ><circle cx="12" cy="12" r="5" /><line x1="12" y1="1" x2="12" y2="3" /><line
              x1="12"
              y1="21"
              x2="12"
              y2="23"
            /><line x1="4.22" y1="4.22" x2="5.64" y2="5.64" /><line
              x1="18.36"
              y1="18.36"
              x2="19.78"
              y2="19.78"
            /><line x1="1" y1="12" x2="3" y2="12" /><line x1="21" y1="12" x2="23" y2="12" /><line
              x1="4.22"
              y1="19.78"
              x2="5.64"
              y2="18.36"
            /><line x1="18.36" y1="5.64" x2="19.78" y2="4.22" /></svg
          >
        {:else}
          <svg
            viewBox="0 0 24 24"
            width="16"
            height="16"
            fill="none"
            stroke="currentColor"
            stroke-width="2"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" /></svg
          >
        {/if}
      </button>
      <button
        class="sidebar-icon-btn"
        onclick={toggleCollapse}
        title={collapsed ? t('sidebar.expand') : t('sidebar.collapse')}
      >
        <svg
          viewBox="0 0 24 24"
          width="16"
          height="16"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          {#if collapsed}
            <polyline points="9 18 15 12 9 6" />
          {:else}
            <polyline points="15 18 9 12 15 6" />
          {/if}
        </svg>
      </button>
    </div>
    <a class="sidebar-branding" href="https://www.seeseo.fr" target="_blank" rel="noopener" style="display:none">
      {#if isDark}
        <svg viewBox="0 0 224 213" width="16" height="16"
          ><g fill="#fff" fill-rule="evenodd" clip-rule="evenodd"
            ><circle cx="9.2" cy="37.5" r="9.2" /><circle cx="13.2" cy="11.5" r="8.2" /><circle
              cx="40.8"
              cy="50.9"
              r="15.8"
            /><path
              d="M219.7 199.8l-42.1-42C190.9 141.1 199 120 199 97c0-53.6-43.4-97-97-97C76.8 0 53.9 9.6 36.6 25.3c1.3-.2 2.7-.3 4.1-.3 6.5 0 12.5 2.5 17 6.6C70.3 23 85.5 18 102 18c43.6 0 79 35.4 79 79s-35.4 79-79 79-79-35.4-79-79c0-9 1.5-17.7 4.3-25.8-6.4-5-10.8-11.8-11.5-19.7C8.9 65.8 5 81 5 97c0 53.6 43.4 97 97 97 19.9 0 38.4-6 53.8-16.3l43 43c3.9 3.9 11.5 2.6 17-2.9l1-1c5.5-5.5 6.7-13.1 2.9-17z"
            /><path d="M56 124l34-28 4 5-35 28-3-5z" /><path
              d="M106 99l24 28-5 3-22-27 3-4z"
            /><path d="M96 82l12-39 4 2-11 39-5-2z" /><path
              d="M156 85l-20 44 4 2 20-45-4-1z"
            /><path d="M153 72l-36-22-2 4 35 22 3-4z" /><path
              d="M90 83L56 58l-3 4 33 24 4-3z"
            /><path d="M106 90l44-10v5l-44 10v-5z" /><path
              d="M87.7 79l18.7.4c2.9.1 5.1 2.5 5.1 5.3l-.5 18.7c0 2.8-2.4 5.1-5.3 5l-18.7-.4c-2.8-.1-5.1-2.4-5-5.3l.4-18.6c.1-2.9 2.4-5.1 5.3-5.1z"
            /><circle cx="53.5" cy="129.8" r="12.5" /><circle
              cx="110.5"
              cy="45.8"
              r="12.5"
            /><circle cx="155.8" cy="80.5" r="12.5" /><circle cx="131.8" cy="132.5" r="12.5" /></g
          ></svg
        >
      {:else}
        <svg viewBox="0 0 224 213" width="16" height="16"
          ><ellipse fill="#fff" cx="97.667" cy="91.346" rx="91.333" ry="89.346" /><circle
            fill="#FF8F00"
            cx="9.167"
            cy="37.5"
            r="9.167"
          /><circle fill="#FF8F00" cx="13.167" cy="11.5" r="8.167" /><circle
            fill="#FF8F00"
            cx="40.75"
            cy="50.916"
            r="15.75"
          /><path
            fill="#FFA300"
            d="M102,23c-15.7,0-30.248,4.903-42.224,13.242C63.2,40.296,65.25,45.421,65.25,51c0,13.117-11.305,23.75-25.25,23.75c-2.856,0-5.599-.453-8.159-1.275C29.363,80.868,28,88.772,28,97c0,40.869,33.131,74,74,74s74-33.131,74-74S142.869,23,102,23z"
          /><path fill="#fff" d="M56,124l34-28l4,5l-35,28L56,124z" /><path
            fill="#fff"
            d="M106,99l24,28l-5,3l-22-27L106,99z"
          /><path fill="#fff" d="M96,82l12-39l4,2l-11,39L96,82z" /><path
            fill="#fff"
            d="M156,85l-20,44l4,2l20-45L156,85z"
          /><path fill="#fff" d="M153,72l-36-22l-2,4l35,22L153,72z" /><path
            fill="#fff"
            d="M90,83L56,58l-3,4l33,24L90,83z"
          /><path fill="#fff" d="M106,90l44-10v5l-44,10V90z" /><path
            fill="#fff"
            d="M87.72,79.005l18.681.433c2.866.066,5.135,2.438,5.067,5.298l-.438,18.64c-.066,2.859-2.444,5.124-5.311,5.058l-18.681-.434c-2.866-.066-5.135-2.438-5.067-5.298l.438-18.64C82.476,81.203,84.854,78.939,87.72,79.005z"
          /><circle fill="#fff" cx="53.5" cy="129.834" r="12.5" /><circle
            fill="#fff"
            cx="110.5"
            cy="45.834"
            r="12.5"
          /><circle fill="#fff" cx="155.833" cy="80.5" r="12.5" /><circle
            fill="#fff"
            cx="131.833"
            cy="132.501"
            r="12.5"
          /><path
            fill="#3D3D3D"
            d="M219.674,199.824l-42.065-42.065C190.988,141.132,199,120.003,199,97c0-53.572-43.429-97-97-97C76.821,0,53.884,9.595,36.642,25.327c1.311-.212,2.654-.327,4.025-.327,6.546,0,12.502,2.519,16.959,6.637C70.275,23.033,85.548,18,102,18c43.631,0,79,35.37,79,79c0,43.631-35.369,79-79,79s-79-35.369-79-79c0-9.056,1.543-17.746,4.349-25.847C20.996,67.145,16.572,60.36,15.792,52.5C8.897,65.828,5,80.958,5,97c0,53.572,43.429,97,97,97c19.921,0,38.438-6.009,53.842-16.309l42.982,42.982c3.905,3.904,11.516,2.625,16.999-2.857l.993-.993C222.299,211.34,223.578,203.729,219.674,199.824z"
          /></svg
        >
      {/if}
      {#if !collapsed}{t('sidebar.byBrand')}{/if}
    </a>
    {#if appVersion && !collapsed}
      <span class="sidebar-version">v{appVersion}</span>
    {/if}
  </div>
</aside>

<style>
  .sidebar {
    width: var(--sidebar-width);
    background: var(--bg-sidebar);
    border-right: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    position: fixed;
    top: var(--topbar-height);
    left: 0;
    bottom: 0;
    z-index: 100;
    overflow-y: auto;
    overflow-x: hidden;
    transition: width 0.2s ease;
  }
  .sidebar-header {
    padding: 20px 20px 16px;
    display: flex;
    align-items: center;
    gap: 12px;
    border-bottom: 1px solid var(--border-light);
  }
  .sidebar-logo {
    width: 36px;
    height: 36px;
    border-radius: var(--radius-sm);
    object-fit: contain;
  }
  .sidebar-logo-placeholder {
    width: 36px;
    height: 36px;
    border-radius: var(--radius-sm);
    background: var(--accent);
    color: var(--accent-text);
    display: flex;
    align-items: center;
    justify-content: center;
    font-weight: 700;
    font-size: 16px;
    flex-shrink: 0;
  }
  svg.sidebar-logo {
    flex-shrink: 0;
  }
  .sidebar-app-name {
    font-weight: 700;
    font-size: 17px;
    color: var(--text);
    letter-spacing: -0.02em;
  }
  .sidebar-section {
    padding: 16px 12px 8px;
  }
  .sidebar-section-title {
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-muted);
    padding: 0 8px 8px;
  }
  .sidebar-add-btn {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    padding: 2px 4px;
    border-radius: 4px;
    display: flex;
    align-items: center;
    transition:
      color 0.15s,
      background 0.15s;
  }
  .sidebar-add-btn:hover {
    color: var(--accent);
    background: var(--bg-hover);
  }
  .sidebar-search {
    padding: 0 8px 6px;
  }
  .sidebar-search-input {
    width: 100%;
    padding: 5px 8px;
    font-size: 12px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg);
    color: var(--text);
    outline: none;
    box-sizing: border-box;
  }
  .sidebar-search-input:focus {
    border-color: var(--accent);
    box-shadow: 0 0 0 2px color-mix(in srgb, var(--accent) 20%, transparent);
  }
  .sidebar-search-input::placeholder {
    color: var(--text-muted);
  }
  .sidebar-inline-input {
    padding: 0 8px 6px;
  }
  .sidebar-inline-input input {
    width: 100%;
    padding: 5px 8px;
    font-size: 12px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg);
    color: var(--text);
    outline: none;
  }
  .sidebar-inline-input input:focus {
    border-color: var(--accent);
    box-shadow: 0 0 0 2px color-mix(in srgb, var(--accent) 20%, transparent);
  }
  .sidebar-nav {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .sidebar-link {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 9px 12px;
    border-radius: var(--radius-sm);
    color: var(--text-secondary);
    font-size: 14px;
    font-weight: 500;
    cursor: pointer;
    transition: all 0.15s;
    border: none;
    background: none;
    width: 100%;
    text-align: left;
  }
  .sidebar-link:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .sidebar-link.active {
    background: var(--accent-light);
    color: var(--accent);
    font-weight: 600;
  }
  .sidebar-link svg {
    width: 18px;
    height: 18px;
    flex-shrink: 0;
    opacity: 0.7;
  }
  .sidebar-link.active svg {
    opacity: 1;
  }
  .sidebar-project {
    margin-bottom: 2px;
  }
  .sidebar-badge {
    font-size: 11px;
    font-weight: 600;
    color: var(--text-muted);
    background: var(--bg-hover);
    padding: 1px 6px;
    border-radius: 8px;
    flex-shrink: 0;
  }
  .sidebar-link.sidebar-project-header {
    font-weight: 500;
    font-size: 14px;
  }
  .sidebar-link.sidebar-view-all {
    font-size: 12px;
    color: var(--accent);
    padding: 6px 12px;
    justify-content: center;
  }
  .sidebar-stats {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: 0 8px;
  }
  .sidebar-stat {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 3px 6px;
    border-radius: var(--radius-sm);
    font-size: 12px;
  }
  .sidebar-stat-label {
    color: var(--text-muted);
    font-weight: 500;
  }
  .sidebar-stat-value {
    color: var(--text-secondary);
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }
  .sidebar-footer {
    margin-top: auto;
    padding: 16px 12px;
    border-top: 1px solid var(--border-light);
  }
  .sidebar-branding {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 9px 12px;
    font-size: 11px;
    color: var(--text-muted);
    text-decoration: none;
    opacity: 0.6;
    transition: opacity 0.15s;
  }
  .sidebar-branding:hover {
    opacity: 1;
  }
  .sidebar-version {
    font-size: 10px;
    color: var(--text-muted);
    opacity: 0.5;
    padding: 4px 12px;
  }
  /* Sidebar collapsible details */
  .sidebar-details {
    padding: 0 12px;
    margin-top: 8px;
  }
  .sidebar-details > summary {
    cursor: pointer;
    user-select: none;
    list-style: none;
  }
  .sidebar-details > summary::-webkit-details-marker {
    display: none;
  }
  .sidebar-details-body {
    padding-top: 4px;
  }
  .sidebar-project-name {
    flex: 1;
  }
  .sidebar-section-push {
    margin-top: auto;
  }
  /* Footer actions row */
  .sidebar-footer-actions {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 4px;
    margin-bottom: 8px;
  }
  .sidebar-icon-btn {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    padding: 7px;
    border-radius: var(--radius-sm);
    display: flex;
    align-items: center;
    transition:
      color 0.15s,
      background 0.15s;
  }
  .sidebar-icon-btn:hover {
    color: var(--text);
    background: var(--bg-hover);
  }

  /* Collapsed state */
  .sidebar.collapsed .sidebar-header {
    padding: 16px 10px;
    justify-content: center;
  }
  .sidebar.collapsed .sidebar-logo {
    width: 28px;
    height: 28px;
  }
  .sidebar.collapsed .sidebar-section {
    padding: 8px 6px 4px;
  }
  .sidebar.collapsed .sidebar-link {
    justify-content: center;
    padding: 9px 0;
  }
  .sidebar.collapsed .sidebar-footer {
    padding: 12px 6px;
  }
  .sidebar.collapsed .sidebar-branding {
    justify-content: center;
    padding: 6px;
  }
  .sidebar.collapsed .sidebar-footer-actions {
    flex-direction: column;
  }

  @media (max-width: 768px) {
    .sidebar {
      display: none;
    }
  }
</style>
