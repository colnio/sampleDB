// Direction C — Lab Notebook: editorial, serif hero, teal+amber
const cStatusChip = (s) => {
  const map = { active:'status-chip--active', progress:'status-chip--progress', archived:'status-chip--archived' };
  const label = s==='progress'?'In Progress':s[0].toUpperCase()+s.slice(1);
  return <span className={`status-chip ${map[s]||''}`} style={{fontFamily:'JetBrains Mono,monospace',textTransform:'uppercase',letterSpacing:'0.08em',fontSize:10.5,padding:'3px 10px'}}>{label}</span>;
};

function C_Ribbon({ screen, setScreen }) {
  const nav = [
    {id:'dashboard', l:'Dashboard'}, {id:'samples', l:'Samples'}, {id:'wiki', l:'Wiki'},
    {id:'booking', l:'Equipment'}, {id:'tokens', l:'Tokens'}, {id:'admin', l:'Admin'},
  ];
  return (
    <div className="top-ribbon">
      <div className="top-ribbon__brand"><Ic.Flask style={{width:26,height:26, color:'var(--c-teal)'}}/><div className="wordmark">SampleDB</div><div className="sub">Lab notebook / v2</div></div>
      <div className="top-ribbon__nav">{nav.map(n=><a key={n.id} className={screen===n.id?'active':''} onClick={()=>setScreen(n.id)}>{n.l}</a>)}</div>
      <div className="top-ribbon__actions">
        <button className="btn btn--sm btn--ghost">⌘K Search</button>
        <div className="user-chip"><div className="av">M</div>msmith</div>
      </div>
    </div>
  );
}

function C_Dashboard({ setScreen, setActiveSample }) {
  return (
    <div className="main">
      <div className="section-head">
        <div className="section-head__eyebrow">Entry · 04.19.26</div>
        <div>
          <h1 className="section-head__title">Good afternoon,<br/>msmith.</h1>
          <p className="section-head__sub">Five samples updated today, two bookings ahead of you, and three access requests waiting.</p>
        </div>
        <div className="section-head__actions"><button className="btn btn--accent" onClick={()=>setScreen('new')}>+ New sample</button></div>
      </div>
      <div className="stats">
        {[['Total samples','247','+12 this week'],['Active bookings','8','2 today'],['Articles','42','+3'],['Pending','3','needs review','down']].map((x,i)=>(
          <div key={i} className="stat-card">
            <div className="stat-card__label">{x[0]}</div>
            <div className="stat-card__value">{x[1]}</div>
            <div className={'stat-card__delta'+(x[3]==='down'?' down':'')}>{x[2]}</div>
          </div>
        ))}
      </div>
      <div style={{display:'grid', gridTemplateColumns:'2fr 1fr', gap: 48}}>
        <div>
          <div className="panel__title">Recent samples</div>
          <table className="data-table">
            <thead><tr><th>ID</th><th>Name</th><th>Status</th><th>Owner</th></tr></thead>
            <tbody>{SAMPLES.slice(0,6).map(s=>(
              <tr key={s.id} style={{cursor:'pointer'}} onClick={()=>{setActiveSample(s); setScreen('detail');}}>
                <td><span className="id">{s.id}</span></td>
                <td><span className="name">{s.name}</span></td>
                <td>{cStatusChip(s.status)}</td>
                <td style={{fontSize:13, color:'var(--c-muted)'}}>{s.owner}</td>
              </tr>
            ))}</tbody>
          </table>
        </div>
        <div>
          <div className="panel__title">Activity log</div>
          {ACTIVITY.slice(0,7).map((a,i)=>(
            <div key={i} className="activity-item">
              <span className="time">{a.when}</span>
              <div><span className="who">{a.who}</span> <span style={{color:'var(--c-muted)'}}>{a.verb}</span> <strong>{a.what}</strong></div>
              <span/>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function C_Samples({ setScreen, setActiveSample }) {
  const [view,setView]=React.useState('grid');
  const [status,setStatus]=React.useState('all');
  const filtered = SAMPLES.filter(s=>status==='all'||s.status===status);
  return (
    <div className="main">
      <div className="section-head">
        <div className="section-head__eyebrow">Index · {SAMPLES.length} entries</div>
        <div>
          <h1 className="section-head__title">Samples.</h1>
          <p className="section-head__sub">Every batch, wafer and flake in the lab. Click to open, hover to peek.</p>
        </div>
        <div className="section-head__actions"><button className="btn btn--accent" onClick={()=>setScreen('new')}>+ New sample</button></div>
      </div>
      <div className="toolbar">
        <div className="search"><Ic.Search/><input placeholder="Search name, ID, keyword…"/></div>
        {['all','active','progress','archived'].map(s=><button key={s} className={'filter-chip'+(status===s?' active':'')} onClick={()=>setStatus(s)}>{s==='all'?'all':s}</button>)}
        <div className="seg">
          <button className={view==='grid'?'active':''} onClick={()=>setView('grid')}><Ic.Grid/>Gallery</button>
          <button className={view==='table'?'active':''} onClick={()=>setView('table')}><Ic.List/>Index</button>
        </div>
      </div>
      {view==='grid' ? (
        <div className="samples-grid">
          {filtered.map(s => (
            <div key={s.id} className="sample-card" onClick={()=>{setActiveSample(s); setScreen('detail');}}>
              <div className="sample-card__thumb"><SampleThumb seed={parseInt(s.id.slice(-3),36)} palette={s.palette}/></div>
              <div className="sample-card__head"><span className="sample-card__id">{s.id}</span>{cStatusChip(s.status)}</div>
              <h3 className="sample-card__title">{s.name}</h3>
              <p className="sample-card__desc">{s.description}</p>
              <div className="sample-card__meta"><span>{s.owner}</span><span>{s.createdAt}</span><span>{s.steps} steps</span></div>
            </div>
          ))}
        </div>
      ) : (
        <table className="data-table">
          <thead><tr><th>ID</th><th>Name</th><th>Status</th><th>Owner</th><th>Steps</th><th>Updated</th></tr></thead>
          <tbody>{filtered.map(s=>(
            <tr key={s.id} style={{cursor:'pointer'}} onClick={()=>{setActiveSample(s); setScreen('detail');}}>
              <td><span className="id">{s.id}</span></td>
              <td><span className="name">{s.name}</span><div style={{fontSize:12,color:'var(--c-muted)',marginTop:2}}>{s.description.slice(0,80)}…</div></td>
              <td>{cStatusChip(s.status)}</td>
              <td style={{color:'var(--c-muted)'}}>{s.owner}</td>
              <td style={{fontFamily:'JetBrains Mono,monospace',fontSize:12,color:'var(--c-muted)'}}>{s.steps}</td>
              <td style={{fontFamily:'JetBrains Mono,monospace',fontSize:12,color:'var(--c-muted)'}}>{s.createdAt}</td>
            </tr>
          ))}</tbody>
        </table>
      )}
    </div>
  );
}

function C_Detail({ sample, setScreen }) {
  const s = sample || SAMPLES[0];
  return (
    <div className="main">
      <div className="back-link" onClick={()=>setScreen('samples')}><Ic.ArrowLeft style={{width:12,height:12}}/> back to samples</div>
      <div className="section-head">
        <div className="section-head__eyebrow">{s.id}</div>
        <div>
          <div style={{marginBottom: 14}}>{cStatusChip(s.status)}</div>
          <h1 className="section-head__title">{s.name}</h1>
          <p className="section-head__sub">{s.description}</p>
        </div>
        <div className="section-head__actions"><button className="btn btn--sm">Share</button><button className="btn btn--accent btn--sm">Edit</button></div>
      </div>
      <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:48}}>
        <div>
          <div className="panel" style={{borderTop:'none', paddingTop:0}}>
            <div className="panel__title">Keywords</div>
            <div style={{display:'flex',gap:8,flexWrap:'wrap'}}>{s.keywords.map(k=><span key={k} style={{fontFamily:'JetBrains Mono,monospace',fontSize:11,padding:'4px 12px',borderRadius:999,border:'1px solid var(--c-line)', textTransform:'uppercase', letterSpacing:'0.08em'}}>{k}</span>)}</div>
          </div>
          <div className="panel">
            <div className="panel__title">Attachments · {s.attachments}</div>
            <div style={{display:'grid',gridTemplateColumns:'repeat(3,1fr)',gap:1,background:'var(--c-line)',border:'1px solid var(--c-line)'}}>
              {Array.from({length:6}).map((_,i)=><div key={i} style={{height:100,overflow:'hidden',background:'var(--c-paper)'}}><SampleThumb seed={i+s.id.length} palette={s.palette}/></div>)}
            </div>
          </div>
        </div>
        <div>
          <div className="panel" style={{borderTop: 'none', paddingTop: 0}}>
            <div className="panel__title">Properties</div>
            {[['Owner',s.owner],['Created',s.createdAt],['Last edit','2h ago'],['Steps',s.steps],['Files',s.attachments]].map(([k,v],i)=>(
              <div key={i} style={{display:'grid',gridTemplateColumns:'120px 1fr',padding:'12px 0',borderBottom:'1px solid var(--c-line)',fontSize:14}}>
                <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:10.5,color:'var(--c-muted)',textTransform:'uppercase',letterSpacing:'0.1em'}}>{k}</span><span>{v}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
      <div className="panel">
        <div className="panel__title">Fabrication — {STEPS.length} steps</div>
        {STEPS.map((st,i)=>(
          <div key={i} className="fab-step">
            <div className="fab-step__num">{String(i+1).padStart(2,'0')}</div>
            <div>
              <div className="fab-step__title">{st.title}</div>
              <div className="fab-step__meta"><span>{st.date}</span><span>by {st.by}</span><span>{st.tool}</span>{st.notes && <span style={{textTransform:'none',letterSpacing:0,fontFamily:'Inter',fontSize:13,color:'var(--c-muted)'}}>{st.notes}</span>}</div>
            </div>
            <div className="fab-step__actions"><button className="btn btn--sm btn--ghost"><Ic.Edit/></button></div>
          </div>
        ))}
        <button className="btn btn--sm" style={{marginTop: 16}}>+ Add step</button>
      </div>
    </div>
  );
}

function C_New({ setScreen }) {
  return (
    <div className="main">
      <div className="back-link" onClick={()=>setScreen('samples')}><Ic.ArrowLeft style={{width:12,height:12}}/> back to samples</div>
      <div className="section-head">
        <div className="section-head__eyebrow">Entry · New</div>
        <div><h1 className="section-head__title">Register a sample.</h1><p className="section-head__sub">Capture the essentials. Fabrication steps come next.</p></div>
      </div>
      <div style={{maxWidth:720,display:'grid',gap:28}}>
        <div className="field"><label>Sample name</label><input placeholder="GaAs Wafer Batch 043"/></div>
        <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:28}}>
          <div className="field"><label>Sample ID</label><input placeholder="SMP-043"/></div>
          <div className="field"><label>Owner</label><select><option>msmith</option><option>— unassigned —</option></select></div>
        </div>
        <div className="field"><label>Description</label><textarea placeholder="Growth conditions, substrate, key parameters…"/></div>
        <div className="field"><label>Keywords</label><input placeholder="GaAs, MBE, heterostructure"/></div>
        <div style={{display:'flex',gap:10,marginTop: 10}}>
          <button className="btn" onClick={()=>setScreen('samples')}>Cancel</button>
          <button className="btn btn--accent">Save & add steps →</button>
        </div>
      </div>
    </div>
  );
}

function C_Wiki({ setScreen }) {
  return (
    <div className="main">
      <div className="section-head">
        <div className="section-head__eyebrow">Knowledge base</div>
        <div><h1 className="section-head__title">Wiki.</h1><p className="section-head__sub">Protocols, equipment guides, and everything you need to not blow up the cleanroom.</p></div>
        <div className="section-head__actions"><button className="btn btn--accent">+ New article</button></div>
      </div>
      <div className="toolbar">
        <div className="search"><Ic.Search/><input placeholder="Search articles…"/></div>
        {['All','Safety','Equipment','Protocols','Admin'].map(t=><button key={t} className={'filter-chip'+(t==='All'?' active':'')}>{t}</button>)}
      </div>
      <div style={{display:'grid',gridTemplateColumns:'repeat(auto-fill,minmax(320px,1fr))',gap:1,background:'var(--c-line)',border:'1px solid var(--c-line)'}}>
        {ARTICLES.map(a=>(
          <div key={a.slug} onClick={()=>setScreen('wikiArticle')} style={{background:'var(--c-paper)',padding:'28px 24px',cursor:'pointer'}}>
            <div style={{fontFamily:'JetBrains Mono,monospace',fontSize:10.5,color:'var(--c-muted)',textTransform:'uppercase',letterSpacing:'0.12em',marginBottom:10}}>{a.tag} · {a.createdAt}</div>
            <h3 style={{fontFamily:'Fraunces,serif',fontSize:24,fontWeight:600,margin:'0 0 10px',letterSpacing:'-0.02em'}}>{a.title}</h3>
            <p style={{margin:0,fontSize:13.5,color:'var(--c-muted)',lineHeight:1.65}}>{a.summary}</p>
            <div style={{marginTop:14,fontSize:12,color:'var(--c-muted)'}}>— {a.author}</div>
          </div>
        ))}
      </div>
    </div>
  );
}

function C_WikiArticle({ setScreen }) {
  return (
    <div className="main">
      <div className="back-link" onClick={()=>setScreen('wiki')}><Ic.ArrowLeft style={{width:12,height:12}}/> back to wiki</div>
      <div className="section-head">
        <div className="section-head__eyebrow">Safety · msmith</div>
        <div><h1 className="section-head__title">Cleanroom<br/>safety checklist.</h1><p className="section-head__sub">Published Nov 2024. Review before every session.</p></div>
        <div className="section-head__actions"><button className="btn btn--sm">Edit</button></div>
      </div>
      <div style={{maxWidth:720,fontSize:16,lineHeight:1.75}}>
        <h2 style={{fontFamily:'Fraunces,serif', fontSize: 28, fontWeight: 600, marginTop: 20, letterSpacing: '-0.02em'}}>Entering the cleanroom</h2>
        <ol style={{lineHeight:1.9}}>
          <li>Sign in at the access terminal using your lab badge.</li>
          <li>Remove outerwear and store in the assigned locker.</li>
          <li>Gown up in this order: booties → bouffant → mask → coverall → gloves.</li>
        </ol>
        <h2 style={{fontFamily:'Fraunces,serif', fontSize: 28, fontWeight: 600, marginTop: 32, letterSpacing: '-0.02em'}}>Chemical handling</h2>
        <p>Any use of HF, KOH, or solvents above 10 mL must be logged in the wet bench registry. Double-glove and confirm the emergency shower is unobstructed.</p>
      </div>
    </div>
  );
}

function C_Booking() {
  const hours = Array.from({length:12},(_,i)=>i+8);
  return (
    <div className="main">
      <div className="section-head">
        <div className="section-head__eyebrow">Schedule · Apr 19, 2026</div>
        <div><h1 className="section-head__title">Equipment.</h1><p className="section-head__sub">Click a lane to book. Drag the block edge to adjust.</p></div>
        <div className="section-head__actions"><button className="btn btn--sm">‹ Prev</button><button className="btn btn--sm">Today</button><button className="btn btn--sm">Next ›</button></div>
      </div>
      <div className="timeline-wrap">
        <div className="tline">
          <div className="tline-head">
            <div className="tline-head__left">Equipment</div>
            <div className="tline-hours">{hours.map(h=><div key={h} className="tline-hour">{String(h).padStart(2,'0')}:00</div>)}</div>
          </div>
          {EQUIPMENT.map(eq=>(
            <div className="tline-row" key={eq.id}>
              <div className="tline-row__left">{eq.name}</div>
              <div className="tline-lane">
                {BOOKINGS.filter(b=>b.eq===eq.id).map((b,i)=>{
                  const left=(b.start-8)*90, w=(b.end-b.start)*90;
                  return <div key={i} className={'tline-block'+(b.own?' tline-block--own':'')} style={{left, width:w-2}}>
                    <div style={{fontWeight:600}}>{b.user}</div><small>{b.purpose}</small>
                  </div>;
                })}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function C_Tokens() {
  return (
    <div className="main">
      <div className="section-head">
        <div className="section-head__eyebrow">Personal access</div>
        <div><h1 className="section-head__title">API tokens.</h1><p className="section-head__sub">Programmatic access for mobile, MCP clients, and lab automations.</p></div>
      </div>
      <div style={{display:'grid',gridTemplateColumns:'1fr 2fr',gap:48}}>
        <div>
          <div className="panel__title">Generate new</div>
          <div className="field" style={{marginBottom:20}}><label>Token name</label><input placeholder="e.g. iOS app"/></div>
          <div className="field" style={{marginBottom:20}}><label>Expiration</label><select><option>30 days</option><option>Never</option></select></div>
          <button className="btn btn--accent">Generate →</button>
        </div>
        <div>
          <div className="panel__title">Existing tokens</div>
          <table className="data-table">
            <thead><tr><th>Name</th><th>Created</th><th>Last used</th><th>Status</th><th></th></tr></thead>
            <tbody>{TOKENS.map(t=>(
              <tr key={t.name}>
                <td><span className="name">{t.name}</span></td>
                <td style={{fontFamily:'JetBrains Mono,monospace',fontSize:12,color:'var(--c-muted)'}}>{t.created}</td>
                <td style={{fontFamily:'JetBrains Mono,monospace',fontSize:12,color:'var(--c-muted)'}}>{t.used}</td>
                <td>{t.status==='active'?cStatusChip('active'):<span style={{color:'var(--c-muted)',fontSize:11.5}}>Revoked</span>}</td>
                <td>{t.status==='active'&&<button className="btn btn--sm">Revoke</button>}</td>
              </tr>
            ))}</tbody>
          </table>
        </div>
      </div>
    </div>
  );
}

function C_Admin() {
  return (
    <div className="main">
      <div className="section-head">
        <div className="section-head__eyebrow">Lab administration</div>
        <div><h1 className="section-head__title">Admin.</h1><p className="section-head__sub">Manage users, groups, and equipment access.</p></div>
      </div>
      <div style={{display:'grid',gridTemplateColumns:'1fr 2fr',gap:48,marginBottom:48}}>
        <div>
          <div className="panel__title">Groups</div>
          {['Barbaros','Ahmet','NewGroup'].map(g=>(
            <div key={g} style={{display:'flex',justifyContent:'space-between',alignItems:'center',padding:'14px 0',borderBottom:'1px solid var(--c-line)'}}>
              <div><div style={{fontFamily:'Fraunces,serif',fontSize:18,fontWeight:600}}>{g}</div><div style={{fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'var(--c-muted)',textTransform:'uppercase',letterSpacing:'0.1em'}}>{g==='Barbaros'?3:g==='Ahmet'?2:1} members</div></div>
              <button className="btn btn--sm btn--ghost"><Ic.Trash/></button>
            </div>
          ))}
          <button className="btn btn--sm" style={{marginTop:16}}>+ Add group</button>
        </div>
        <div>
          <div className="panel__title">Equipment</div>
          <table className="data-table"><thead><tr><th>Name</th><th>Status</th><th>Access</th></tr></thead><tbody>{EQUIPMENT.map(e=><tr key={e.id}><td><span className="name">{e.name}</span></td><td>{cStatusChip('active')}</td><td style={{fontFamily:'JetBrains Mono,monospace',fontSize:11.5,color:'var(--c-muted)'}}>all groups</td></tr>)}</tbody></table>
        </div>
      </div>
      <div>
        <div className="panel__title">Users · {USERS.length}</div>
        <table className="data-table"><thead><tr><th>User</th><th>Group</th><th>Status</th><th>Role</th></tr></thead><tbody>{USERS.map(u=>(
          <tr key={u.user}>
            <td><span className="name">{u.user}</span></td>
            <td style={{color:'var(--c-muted)'}}>{u.group}</td>
            <td>{u.status==='approved'?cStatusChip('active'):<span style={{fontFamily:'JetBrains Mono,monospace',fontSize:10.5,color:'var(--c-amber)',letterSpacing:'0.08em',textTransform:'uppercase'}}>pending</span>}</td>
            <td>{u.admin?<span style={{fontFamily:'JetBrains Mono,monospace',fontSize:10.5,color:'var(--c-teal)',letterSpacing:'0.08em',textTransform:'uppercase'}}>admin</span>:<span style={{color:'var(--c-muted)'}}>member</span>}</td>
          </tr>
        ))}</tbody></table>
      </div>
    </div>
  );
}

function DirectionC({ screen, setScreen, activeSample, setActiveSample }) {
  return (
    <div className="dirC">
      <div className="stage-bg">
        <C_Ribbon screen={screen} setScreen={setScreen}/>
        {screen==='dashboard' && <C_Dashboard setScreen={setScreen} setActiveSample={setActiveSample}/>}
        {screen==='samples' && <C_Samples setScreen={setScreen} setActiveSample={setActiveSample}/>}
        {screen==='detail' && <C_Detail sample={activeSample} setScreen={setScreen}/>}
        {screen==='new' && <C_New setScreen={setScreen}/>}
        {screen==='wiki' && <C_Wiki setScreen={setScreen}/>}
        {screen==='wikiArticle' && <C_WikiArticle setScreen={setScreen}/>}
        {screen==='booking' && <C_Booking/>}
        {screen==='tokens' && <C_Tokens/>}
        {screen==='admin' && <C_Admin/>}
      </div>
    </div>
  );
}

Object.assign(window, { DirectionC });
