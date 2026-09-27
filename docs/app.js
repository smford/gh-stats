// gh-stats Interactive App Script
document.addEventListener('DOMContentLoaded', () => {
  initTheme();
  initTabs();
  initSimulator();
  initCopyButtons();
  initRulesFilter();
});

// Theme Management
function initTheme() {
  const toggleBtn = document.getElementById('theme-toggle');
  const storedTheme = localStorage.getItem('gh-stats-theme');
  const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
  
  const currentTheme = storedTheme || (prefersDark ? 'dark' : 'light');
  document.documentElement.setAttribute('data-theme', currentTheme);
  updateThemeIcon(currentTheme);

  if (toggleBtn) {
    toggleBtn.addEventListener('click', () => {
      const activeTheme = document.documentElement.getAttribute('data-theme') === 'dark' ? 'light' : 'dark';
      document.documentElement.setAttribute('data-theme', activeTheme);
      localStorage.setItem('gh-stats-theme', activeTheme);
      updateThemeIcon(activeTheme);
    });
  }
}

function updateThemeIcon(theme) {
  const icon = document.getElementById('theme-icon');
  if (!icon) return;
  if (theme === 'dark') {
    icon.innerHTML = '<path d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>';
  } else {
    icon.innerHTML = '<path d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" fill="currentColor"/>';
  }
}

// Tab Switching
function initTabs() {
  const tabButtons = document.querySelectorAll('.tab-btn');
  tabButtons.forEach(btn => {
    btn.addEventListener('click', () => {
      const targetId = btn.dataset.tab;
      
      // Update buttons
      tabButtons.forEach(b => b.classList.remove('active'));
      btn.classList.add('active');

      // Update contents
      document.querySelectorAll('.tab-content').forEach(content => {
        content.classList.remove('active');
      });
      const targetContent = document.getElementById(targetId);
      if (targetContent) {
        targetContent.classList.add('active');
      }
    });
  });
}

// Interactive PR Risk Simulator
function initSimulator() {
  const additionsInput = document.getElementById('sim-additions');
  const deletionsInput = document.getElementById('sim-deletions');
  const testsInput = document.getElementById('sim-tests');
  const cicdToggle = document.getElementById('sim-cicd');
  const dbToggle = document.getElementById('sim-db');
  const authToggle = document.getElementById('sim-auth');

  if (!additionsInput) return;

  function calculateRisk() {
    const additions = parseInt(additionsInput.value, 10) || 0;
    const deletions = parseInt(deletionsInput.value, 10) || 0;
    const testLines = parseInt(testsInput.value, 10) || 0;
    const hasCICD = cicdToggle ? cicdToggle.checked : false;
    const hasDB = dbToggle ? dbToggle.checked : false;
    const hasAuth = authToggle ? authToggle.checked : false;

    // Display values
    document.getElementById('val-additions').textContent = `+${additions}`;
    document.getElementById('val-deletions').textContent = `-${deletions}`;
    document.getElementById('val-tests').textContent = `+${testLines}`;

    let score = 10;
    const totalLines = additions + deletions;

    // Size penalty
    if (totalLines > 1000) score += 30;
    else if (totalLines > 600) score += 20;
    else if (totalLines > 300) score += 10;

    // Blast radius penalty
    let sensitiveCount = 0;
    if (hasCICD) { score += 25; sensitiveCount++; }
    if (hasDB) { score += 25; sensitiveCount++; }
    if (hasAuth) { score += 15; sensitiveCount++; }

    // Test coverage ratio
    const codeAdded = Math.max(0, additions - testLines);
    if (codeAdded > 100) {
      const testRatio = testLines / Math.max(1, codeAdded);
      if (testRatio < 0.1) score += 25;
      else if (testRatio < 0.2) score += 15;
    }

    score = Math.min(100, Math.max(10, score));

    // Determine Level
    let level = 'LOW';
    let badgeClass = 'risk-low';
    if (score >= 80) { level = 'CRITICAL'; badgeClass = 'risk-crit'; }
    else if (score >= 60) { level = 'HIGH'; badgeClass = 'risk-high'; }
    else if (score >= 30) { level = 'MEDIUM'; badgeClass = 'risk-med'; }

    // Update UI elements
    const scoreVal = document.getElementById('risk-score-val');
    const levelBadge = document.getElementById('risk-level-badge');
    const previewBox = document.getElementById('simulator-output');

    if (scoreVal) scoreVal.textContent = `${score} / 100`;
    if (levelBadge) {
      levelBadge.textContent = level;
      levelBadge.className = `risk-level-badge ${badgeClass}`;
    }

    if (previewBox) {
      previewBox.textContent = `### 📊 Pull Request SRE Assessment
- Risk Rating: ${level} (Score: ${score}/100)
- Changes: +${additions} / -${deletions} lines
- Test Delta: +${testLines} test lines vs +${codeAdded} production lines
- Sensitive Files: ${sensitiveCount} detected (${hasCICD ? 'CI/CD, ' : ''}${hasDB ? 'DB Migrations, ' : ''}${hasAuth ? 'Auth' : ''})

SARIF Rule Fired:
• GHSTATS001-PR-SUMMARY [${level === 'CRITICAL' || level === 'HIGH' ? 'WARNING' : 'NOTE'}]
${score >= 60 ? '• GHSTATS004-PR-BLAST-RADIUS [WARNING] High blast radius detected\n' : ''}${codeAdded > 100 && testLines === 0 ? '• GHSTATS003-PR-TEST-RATIO [WARNING] Missing test coverage delta\n' : ''}`;
    }
  }

  [additionsInput, deletionsInput, testsInput].forEach(inp => {
    inp.addEventListener('input', calculateRisk);
  });
  [cicdToggle, dbToggle, authToggle].forEach(tog => {
    if (tog) tog.addEventListener('change', calculateRisk);
  });

  calculateRisk();
}

// Copy to Clipboard Buttons
function initCopyButtons() {
  document.querySelectorAll('.copy-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      const code = btn.dataset.copy || btn.parentElement.innerText;
      navigator.clipboard.writeText(code.trim()).then(() => {
        const originalHTML = btn.innerHTML;
        btn.innerHTML = '✓ Copied!';
        btn.style.color = '#10b981';
        setTimeout(() => {
          btn.innerHTML = originalHTML;
          btn.style.color = '';
        }, 1800);
      });
    });
  });
}

// Rules Search Filter
function initRulesFilter() {
  const searchInput = document.getElementById('rules-search');
  if (!searchInput) return;

  searchInput.addEventListener('input', () => {
    const query = searchInput.value.toLowerCase();
    const rows = document.querySelectorAll('#rules-table tbody tr');

    rows.forEach(row => {
      const text = row.textContent.toLowerCase();
      row.style.display = text.includes(query) ? '' : 'none';
    });
  });
}
