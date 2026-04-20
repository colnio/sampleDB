// Main App — depends on window.Ic, window.DirectionA/B/C
const { useState: useStateA, useEffect: useEffectA } = React;

const SCREENS = [
  ['login','Login'], ['dashboard','Dashboard'], ['samples','Samples'],
  ['new','New Sample'],
  ['wiki','Wiki'],
  ['booking','Booking'], ['tokens','Tokens'], ['admin','Admin'],
];

function LoginScreen({ dir, setScreen, theme }) {
  const accent = dir==='A' ? '#2563eb' : dir==='B' ? '#6366f1' : '#0e7c66';
  const isDark = theme === 'dark';
  const bg = isDark ? '#0d1117' : '#f8fafc';
  const panel = isDark ? '#161b22' : '#ffffff';
  const text = isDark ? '#e8e8e8' : '#0b1120';
  const muted = isDark ? '#9ca3a4' : '#64748b';
  const inputBg = isDark ? '#0d1117' : '#ffffff';
  const inputBrd = isDark ? 'rgba(255,255,255,0.12)' : 'rgba(15,23,42,0.12)';
  return (
    <div style={{minHeight:'calc(100vh - 50px)', background: bg, display:'flex', alignItems:'center', justifyContent:'center', padding: 24, fontFamily: dir==='C' ? '"Fraunces", Georgia, serif' : 'Inter, -apple-system, sans-serif'}}>
      <div style={{width: 460, background: panel, borderRadius: 16, padding: 36, boxShadow: isDark ? '0 20px 60px rgba(0,0,0,0.45)' : '0 20px 60px rgba(15,23,42,0.12)', border: `1px solid ${inputBrd}`, borderTop: dir==='A' ? `3px solid ${accent}` : `1px solid ${inputBrd}`}}>
        <div style={{display:'flex', alignItems:'center', gap: 12, marginBottom: 6}}>
          <Ic.Flask style={{width: 34, height: 34, color: accent}}/>
          <div>
            <div style={{fontWeight: 800, fontSize: 18, color: text, letterSpacing: '-0.01em'}}>SampleDB</div>
            <div style={{fontSize: 11, color: muted, fontFamily: '"JetBrains Mono", monospace', textTransform:'uppercase', letterSpacing:'0.1em'}}>Lab portal · v2</div>
          </div>
        </div>
        <h1 style={{fontSize: 32, fontWeight: 800, color: accent, margin: '26px 0 6px', letterSpacing: '-0.02em'}}>Welcome back</h1>
        <p style={{color: muted, fontSize: 14, margin: '0 0 26px', lineHeight: 1.55}}>Sign in to manage samples, wiki pages, and equipment bookings.</p>
        <div style={{display:'flex', flexDirection:'column', gap: 14}}>
          <div>
            <label style={{fontSize: 11, fontWeight: 700, color: muted, display:'block', marginBottom: 8, textTransform:'uppercase', letterSpacing: '0.08em'}}>Username</label>
            <input defaultValue="msmith" style={{width:'100%', padding: '11px 14px', borderRadius: 8, border: `1px solid ${inputBrd}`, background: inputBg, color: text, fontSize: 14, fontFamily:'inherit', boxSizing:'border-box'}}/>
          </div>
          <div>
            <label style={{fontSize: 11, fontWeight: 700, color: muted, display:'block', marginBottom: 8, textTransform:'uppercase', letterSpacing: '0.08em'}}>Password</label>
            <input type="password" defaultValue="passpasspass" style={{width:'100%', padding: '11px 14px', borderRadius: 8, border: `1px solid ${inputBrd}`, background: inputBg, color: text, fontSize: 14, fontFamily:'inherit', boxSizing:'border-box'}}/>
          </div>
          <button onClick={()=>setScreen('dashboard')} style={{padding: '13px', borderRadius: 8, background: accent, color: '#fff', border: 'none', fontSize: 14, fontWeight: 700, cursor:'pointer', marginTop: 8, letterSpacing: '-0.01em'}}>Sign in →</button>
        </div>
        <div style={{textAlign:'center', marginTop: 22, fontSize: 13, color: muted}}>Need an account? <a style={{color: accent, fontWeight: 600, cursor:'pointer'}}>Request access</a></div>
      </div>
    </div>
  );
}

function App() {
  const [dir, setDir] = useStateA(() => localStorage.getItem('sdb_dir') || 'A');
  const [screen, setScreen] = useStateA(() => localStorage.getItem('sdb_screen') || 'dashboard');
  const [theme, setTheme] = useStateA(() => localStorage.getItem('sdb_theme') || 'dark');
  const [activeSample, setActiveSample] = useStateA(null);

  useEffectA(() => { localStorage.setItem('sdb_dir', dir); }, [dir]);
  useEffectA(() => { localStorage.setItem('sdb_screen', screen); }, [screen]);
  useEffectA(() => {
    localStorage.setItem('sdb_theme', theme);
    document.documentElement.setAttribute('data-theme', theme);
  }, [theme]);

  const directionLabels = { A:'Refined', B:'Dense', C:'Editorial' };
  const directionSubs   = { A:'Classic blue, app shell, polished', B:'Linear-inspired, dense, table-first', C:'Lab notebook, serif + teal' };

  const renderScreen = () => {
    if (screen === 'login') return <LoginScreen dir={dir} setScreen={setScreen} theme={theme}/>;
    const props = { screen, setScreen, activeSample, setActiveSample };
    if (dir === 'A') return <DirectionA {...props}/>;
    if (dir === 'B') return <DirectionB {...props}/>;
    return <DirectionC {...props}/>;
  };

  return (
    <React.Fragment>
      <div className="direction-bar">
        <Ic.Flask style={{width:20,height:20, color:'#60a5fa'}}/>
        <span style={{fontWeight:700, letterSpacing:'-0.01em'}}>SampleDB redesign</span>
        <span className="direction-bar__label" style={{marginLeft: 8}}>Direction</span>
        <div className="direction-bar__segs">
          {['A','B','C'].map(k => (
            <button key={k} className={'direction-bar__seg' + (dir===k?' active':'')} onClick={()=>setDir(k)}>
              {k} · {directionLabels[k]}
            </button>
          ))}
        </div>
        <span style={{fontSize: 11, color: 'rgba(255,255,255,0.55)', marginLeft: 2, maxWidth: 220}}>{directionSubs[dir]}</span>
        <div className="direction-bar__screens">
          {SCREENS.map(([k,l]) => (
            <button key={k} className={screen===k?'active':''} onClick={()=>setScreen(k)}>{l}</button>
          ))}
        </div>
        <button className="direction-bar__theme" onClick={()=>setTheme(t=>t==='dark'?'light':'dark')}>
          {theme==='dark' ? <Ic.Sun style={{width:14,height:14}}/> : <Ic.Moon style={{width:14,height:14}}/>}
          {theme==='dark' ? 'Light' : 'Dark'}
        </button>
      </div>
      <div className="stage" data-dir={dir} data-theme={theme}>{renderScreen()}</div>
    </React.Fragment>
  );
}

ReactDOM.createRoot(document.getElementById('root')).render(<App/>);
