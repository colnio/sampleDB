// Direction B — Linear-inspired: dense, monospace, subtle
const bStatusChip = (s) => {
  const map = { active:['#10b981','Active'], progress:['#6366f1','In Progress'], archived:['var(--text-muted)','Archived'], pending:['#f59e0b','Pending'] };
  const [c, l] = map[s] || map.active;
  return <span style={{display:'inline-flex',alignItems:'center',gap:5,fontSize:11.5,fontWeight:500,color:c}}><span style={{width:6,height:6,borderRadius:'50%',background:c}}/>{l}</span>;
};

function B_Sidebar({ screen, setScreen }) {
  const items = [
    { id:'dashboard', label:'Dashboard', icon:Ic.Dashboard },
    { id:'samples',   label:'Samples',   icon:Ic.Samples, count: 247 },
    { id:'wiki',      label:'Wiki',      icon:Ic.Wiki, count: 42 },
    { id:'booking',   label:'Equipment', icon:Ic.Calendar },
    { id:'tokens',    label:'API Tokens',icon:Ic.Key },
    { id:'admin',     label:'Admin',     icon:Ic.Shield },
  ];
  return (
    <aside className="sidebar">
      <div className="sidebar__brand"><div className="mark">S</div><span>SampleDB</span><Ic.Caret style={{marginLeft:'auto',width:12,color:'var(--text-muted)'}}/></div>
      <div className="cmdk"><span>Jump to…</span><span><span className="kbd">⌘</span> <span className="kbd">K</span></span></div>
      <div className="nav-group">
        {items.map(it => (
          <button key={it.id} className={'nav-link'+(screen===it.id?' active':'')} onClick={()=>setScreen(it.id)}>
            <it.icon/><span>{it.label}</span>{it.count && <span className="count">{it.count}</span>}
          </button>
        ))}
      </div>
      <div className="nav-group">
        <div className="nav-group__title">Views</div>
        <button className="nav-link"><Ic.Dot/><span>My samples</span></button>
        <button className="nav-link"><Ic.Dot/><span>Shared with me</span></button>
        <button className="nav-link"><Ic.Dot/><span>Archive</span></button>
      </div>
      <div className="sidebar__footer"><div className="avatar">M</div><span>msmith</span></div>
    </aside>
  );
}

function B_Topbar({ title, crumbs, actions }) {
  return (
    <div className="topbar">
      <div className="crumbs">
        <span>Workspace</span><span className="sep">/</span>
        <h1>{title}</h1>
        {crumbs && <><span className="sep">·</span><span>{crumbs}</span></>}
      </div>
      <div className="topbar__actions">
        {actions || (
          <><button className="btn"><Ic.Filter/> Filter</button>
          <button className="btn btn--primary"><Ic.Plus/> New</button></>
        )}
      </div>
    </div>
  );
}

function B_Dashboard({ setScreen, setActiveSample }) {
  return (
    <>
      <B_Topbar title="Dashboard"/>
      <div className="sub-toolbar"><span style={{fontSize:12.5,color:'var(--text-muted)'}}>Friday · April 19, 2026</span></div>
      <div className="content">
        <div className="stats">
          {[['Samples','247','+12 wk'],['Bookings today','8','2 upcoming'],['Articles','42','+3 wk'],['Pending','3','needs review']].map((x,i)=>(
            <div key={i} className="stat-card">
              <div className="stat-card__label"><Ic.Dot style={{width:8,height:8,color:'var(--b-accent)'}}/>{x[0]}</div>
              <div className="stat-card__value">{x[1]}</div>
              <div className="stat-card__delta">{x[2]}</div>
            </div>
          ))}
        </div>
        <div style={{display:'grid', gridTemplateColumns:'2fr 1fr', gap: 16}}>
          <div>
            <div className="u-between" style={{marginBottom:10}}><strong style={{fontSize:13}}>Recent samples</strong><a style={{fontSize:12,cursor:'pointer'}} onClick={()=>setScreen('samples')}>View all</a></div>
            <table className="data-table">
              <thead><tr><th>ID</th><th>Name</th><th>Status</th><th>Owner</th><th>Updated</th></tr></thead>
              <tbody>
                {SAMPLES.slice(0,6).map(s=>(
                  <tr key={s.id} style={{cursor:'pointer'}} onClick={()=>{setActiveSample(s); setScreen('detail');}}>
                    <td><span className="id">{s.id}</span></td>
                    <td><span className="name">{s.name}</span></td>
                    <td>{bStatusChip(s.status)}</td>
                    <td>{s.owner}</td>
                    <td style={{fontFamily:'JetBrains Mono,monospace',fontSize:11.5,color:'var(--text-muted)'}}>{s.createdAt}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div>
            <div className="u-between" style={{marginBottom:10}}><strong style={{fontSize:13}}>Activity</strong></div>
            <div style={{background:'var(--surface-default)', border:'1px solid var(--b-border)', borderRadius: 8, padding: 6}}>
              {ACTIVITY.slice(0,8).map((a,i)=>(
                <div key={i} className="activity-item">
                  <span className="who">{a.who}</span>
                  <span style={{color:'var(--text-muted)'}}>{a.verb}</span>
                  <a style={{color:'var(--b-accent)'}}>{a.what}</a>
                  <span className="time">{a.when}</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </>
  );
}

function B_Samples({ setScreen, setActiveSample }) {
  const [view, setView] = React.useState('table');
  const [status, setStatus] = React.useState('all');
  const filtered = SAMPLES.filter(s => status==='all' || s.status === status);
  return (
    <>
      <B_Topbar title="Samples" crumbs={`${filtered.length} of ${SAMPLES.length}`}
        actions={<><button className="btn"><Ic.Download/> Export</button><button className="btn btn--primary" onClick={()=>setScreen('new')}><Ic.Plus/> New Sample</button></>}/>
      <div className="sub-toolbar">
        <div className="search"><Ic.Search/><input placeholder="Search samples…"/></div>
        <div className="filter-chip"><Ic.Filter/> Owner</div>
        <div className="filter-chip"><Ic.Calendar/> Apr 1 – 19</div>
        {['all','active','progress','archived'].map(s => (
          <button key={s} className={'filter-chip'+(status===s?' active':'')} onClick={()=>setStatus(s)} style={{border:'none'}}>
            {s==='all'?'Any status':s[0].toUpperCase()+s.slice(1)}
          </button>
        ))}
        <div style={{flex:1}}/>
        <div className="seg">
          <button className={view==='table'?'active':''} onClick={()=>setView('table')}><Ic.List/>Table</button>
          <button className={view==='grid'?'active':''} onClick={()=>setView('grid')}><Ic.Grid/>Grid</button>
          <button className={view==='kanban'?'active':''} onClick={()=>setView('kanban')}><Ic.Kanban/>Board</button>
        </div>
      </div>
      <div className="content">
        {view==='table' && (
          <table className="data-table">
            <thead><tr><th style={{width:28}}><input type="checkbox"/></th><th>ID</th><th>Name</th><th>Status</th><th>Owner</th><th>Steps</th><th>Files</th><th>Keywords</th><th>Updated</th></tr></thead>
            <tbody>
              {filtered.map(s => (
                <tr key={s.id} style={{cursor:'pointer'}} onClick={()=>{setActiveSample(s); setScreen('detail');}}>
                  <td><input type="checkbox" onClick={e=>e.stopPropagation()}/></td>
                  <td><span className="id">{s.id}</span></td>
                  <td><span className="name">{s.name}</span></td>
                  <td>{bStatusChip(s.status)}</td>
                  <td>{s.owner}</td>
                  <td style={{fontFamily:'JetBrains Mono,monospace',fontSize:12,color:'var(--text-muted)'}}>{s.steps}</td>
                  <td style={{fontFamily:'JetBrains Mono,monospace',fontSize:12,color:'var(--text-muted)'}}>{s.attachments}</td>
                  <td><div style={{display:'flex',gap:4,flexWrap:'wrap'}}>{s.keywords.slice(0,2).map(k=><span key={k} style={{fontSize:11,padding:'1px 7px',borderRadius:4,background:'var(--surface-muted)',color:'var(--text-muted)',fontFamily:'JetBrains Mono,monospace'}}>{k}</span>)}</div></td>
                  <td style={{fontFamily:'JetBrains Mono,monospace',fontSize:11.5,color:'var(--text-muted)'}}>{s.createdAt}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        {view==='grid' && (
          <div className="samples-grid">
            {filtered.map(s=>(
              <div key={s.id} className="sample-card" onClick={()=>{setActiveSample(s); setScreen('detail');}}>
                <div className="sample-card__head">
                  <span className="sample-card__id">{s.id}</span>
                  {bStatusChip(s.status)}
                </div>
                <h3 className="sample-card__title">{s.name}</h3>
                <p className="sample-card__desc">{s.description}</p>
                <div className="sample-card__meta"><span>{s.owner}</span><span>·</span><span style={{fontFamily:'JetBrains Mono,monospace'}}>{s.createdAt}</span><span style={{marginLeft:'auto'}}>{s.steps} steps · {s.attachments} files</span></div>
              </div>
            ))}
          </div>
        )}
        {view==='kanban' && (
          <div className="kanban">
            {['active','progress','archived'].map(st => (
              <div key={st} className="kan-col">
                <div className="kan-col__title"><span style={{width:6,height:6,borderRadius:'50%',background: st==='active'?'#10b981':st==='progress'?'#6366f1':'var(--text-muted)'}}/>{st} · {SAMPLES.filter(s=>s.status===st).length}</div>
                {SAMPLES.filter(s=>s.status===st).map(s=>(
                  <div key={s.id} className="kan-item" onClick={()=>{setActiveSample(s); setScreen('detail');}} style={{cursor:'pointer'}}>
                    <div className="id">{s.id}</div>
                    <div className="title">{s.name}</div>
                    <div style={{fontSize:11.5, color:'var(--text-muted)'}}>{s.owner} · {s.steps} steps</div>
                  </div>
                ))}
              </div>
            ))}
          </div>
        )}
      </div>
    </>
  );
}

function B_Detail({ sample, setScreen }) {
  const s = sample || SAMPLES[0];
  return (
    <>
      <B_Topbar title={s.name} crumbs={s.id} actions={<><button className="btn btn--ghost btn--sm"><Ic.ArrowLeft/> Back</button><button className="btn"><Ic.Edit/> Edit</button><button className="btn btn--primary"><Ic.Sparkle/> Share</button></>}/>
      <div className="content">
        <div style={{display:'grid', gridTemplateColumns: '2fr 1fr', gap: 16}}>
          <div style={{display:'flex',flexDirection:'column',gap:16}}>
            <div style={{background:'var(--surface-default)',border:'1px solid var(--b-border)',borderRadius:8,padding:16}}>
              <div className="u-eyebrow" style={{marginBottom:8}}>Description</div>
              <p style={{margin:0,fontSize:13.5,lineHeight:1.65}}>{s.description}</p>
              <div style={{display:'flex',gap:5,marginTop:12,flexWrap:'wrap'}}>{s.keywords.map(k=><span key={k} style={{fontSize:11,padding:'2px 8px',borderRadius:4,background:'var(--surface-muted)',color:'var(--text-muted)',fontFamily:'JetBrains Mono,monospace'}}>{k}</span>)}</div>
            </div>
            <div style={{background:'var(--surface-default)',border:'1px solid var(--b-border)',borderRadius:8,padding:'12px 8px'}}>
              <div className="u-between" style={{padding:'2px 10px 10px'}}>
                <div className="u-eyebrow">Fabrication · {STEPS.length} steps</div>
                <button className="btn btn--sm"><Ic.Plus/> Step</button>
              </div>
              {STEPS.map((st,i)=>(
                <div key={i} className="fab-step">
                  <span className="fab-step__grab">⋮⋮</span>
                  <div className="fab-step__num">{String(i+1).padStart(2,'0')}</div>
                  <div>
                    <div className="fab-step__title">{st.title}</div>
                    <div className="fab-step__meta">{st.date} · {st.by} · {st.tool}{st.notes?' · '+st.notes:''}</div>
                  </div>
                  <div style={{display:'flex',gap:4,opacity:0.6}}>
                    <button className="btn btn--ghost btn--icon btn--sm"><Ic.Edit/></button>
                  </div>
                </div>
              ))}
            </div>
            <div style={{background:'var(--surface-default)',border:'1px solid var(--b-border)',borderRadius:8,padding:16}}>
              <div className="u-between" style={{marginBottom:12}}><div className="u-eyebrow">Attachments · {s.attachments}</div><button className="btn btn--sm"><Ic.Upload/> Upload</button></div>
              <div style={{display:'grid',gridTemplateColumns:'repeat(auto-fill,minmax(120px,1fr))',gap:8}}>
                {Array.from({length:5}).map((_,i)=><div key={i} style={{height:80,borderRadius:6,overflow:'hidden',border:'1px solid var(--b-border)'}}><SampleThumb seed={i+s.id.length} palette={s.palette}/></div>)}
              </div>
            </div>
          </div>
          <div style={{display:'flex',flexDirection:'column',gap:16}}>
            <div style={{background:'var(--surface-default)',border:'1px solid var(--b-border)',borderRadius:8,padding:16}}>
              <div className="u-eyebrow" style={{marginBottom:12}}>Properties</div>
              {[['Status',bStatusChip(s.status)],['Owner',s.owner],['Created',s.createdAt],['Steps',s.steps],['Files',s.attachments]].map(([k,v],i)=>(
                <div key={i} style={{display:'grid',gridTemplateColumns:'80px 1fr',padding:'5px 0',fontSize:12.5,borderBottom:'1px solid var(--b-border)'}}>
                  <span style={{color:'var(--text-muted)'}}>{k}</span><span>{v}</span>
                </div>
              ))}
            </div>
            <div style={{background:'var(--surface-default)',border:'1px solid var(--b-border)',borderRadius:8,padding:'12px 8px'}}>
              <div className="u-eyebrow" style={{padding:'4px 10px 8px'}}>Activity</div>
              {ACTIVITY.slice(0,6).map((a,i)=>(
                <div key={i} className="activity-item">
                  <span className="who">{a.who}</span>
                  <span style={{color:'var(--text-muted)'}}>{a.verb.split(' ')[0]}</span>
                  <span className="time">{a.when}</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </>
  );
}

function B_New({ setScreen }) {
  return (
    <>
      <B_Topbar title="New Sample" crumbs="Step 1 of 2" actions={<><button className="btn" onClick={()=>setScreen('samples')}>Cancel</button><button className="btn btn--primary">Create & add steps</button></>}/>
      <div className="content">
        <div style={{maxWidth:720, background:'var(--surface-default)',border:'1px solid var(--b-border)',borderRadius:8,padding:24}}>
          <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:16}}>
            <div className="field"><label>Sample ID</label><input placeholder="SMP-043"/></div>
            <div className="field"><label>Name</label><input placeholder="GaAs Wafer Batch 043"/></div>
          </div>
          <div className="field" style={{marginTop:12}}><label>Description</label><textarea placeholder="Growth conditions, substrate, key parameters…"/></div>
          <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:16,marginTop:12}}>
            <div className="field"><label>Owner</label><select><option>msmith</option><option>-- Unassigned --</option></select></div>
            <div className="field"><label>Status</label><select><option>Active</option><option>In Progress</option></select></div>
          </div>
          <div className="field" style={{marginTop:12}}><label>Keywords</label><input placeholder="GaAs, MBE"/></div>
        </div>
      </div>
    </>
  );
}

function B_Wiki({ setScreen }) {
  return (
    <>
      <B_Topbar title="Wiki" crumbs={`${ARTICLES.length} articles`} actions={<><button className="btn btn--primary"><Ic.Plus/>New Article</button></>}/>
      <div className="sub-toolbar">
        <div className="search"><Ic.Search/><input placeholder="Search articles…"/></div>
        {['All','Safety','Equipment','Protocols','Admin'].map(t=><span key={t} className={'filter-chip'+(t==='All'?' active':'')}>{t}</span>)}
      </div>
      <div className="content">
        <table className="data-table">
          <thead><tr><th>Title</th><th>Category</th><th>Author</th><th>Updated</th></tr></thead>
          <tbody>
            {ARTICLES.map(a => (
              <tr key={a.slug} onClick={()=>setScreen('wikiArticle')} style={{cursor:'pointer'}}>
                <td><span className="name">{a.title}</span><div style={{fontSize:12,color:'var(--text-muted)',marginTop:2}}>{a.summary}</div></td>
                <td><span style={{fontSize:11.5,padding:'2px 8px',borderRadius:4,background:'var(--b-accent-soft)',color:'var(--b-accent)',fontFamily:'JetBrains Mono,monospace'}}>{a.tag}</span></td>
                <td>{a.author}</td>
                <td style={{fontFamily:'JetBrains Mono,monospace',fontSize:11.5,color:'var(--text-muted)'}}>{a.createdAt}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}

function B_WikiArticle({ setScreen }) {
  return (
    <>
      <B_Topbar title="Cleanroom Safety Checklist" crumbs="Safety" actions={<><button className="btn btn--ghost btn--sm" onClick={()=>setScreen('wiki')}><Ic.ArrowLeft/>Back</button><button className="btn"><Ic.Edit/>Edit</button></>}/>
      <div className="content">
        <div style={{maxWidth: 780, margin: '0 auto'}}>
          <div style={{fontSize: 12, color:'var(--text-muted)', fontFamily:'JetBrains Mono,monospace', marginBottom: 20}}>msmith · Updated Nov 10, 2024</div>
          <h2 style={{fontSize: 20, marginTop: 0}}>Entering the cleanroom</h2>
          <ol style={{lineHeight: 1.8, fontSize: 14}}>
            <li>Sign in at the access terminal using your lab badge.</li>
            <li>Remove outerwear and store in the assigned locker.</li>
            <li>Gown up in this order: booties → bouffant → mask → coverall → gloves.</li>
          </ol>
          <h2 style={{fontSize: 20}}>Chemical handling</h2>
          <p style={{fontSize: 14, lineHeight: 1.7}}>Any use of HF, KOH, or solvents above 10 mL must be logged in the wet bench registry. Double-glove and confirm the emergency shower is unobstructed.</p>
          <pre style={{background:'var(--surface-muted)',padding:14,borderRadius:6,fontSize:12.5,border:'1px solid var(--b-border)'}}>{`# Wet-bench checklist
[ ] PPE verified
[ ] Ventilation on
[ ] Spill kit accessible`}</pre>
        </div>
      </div>
    </>
  );
}

function B_Booking() {
  const hours = Array.from({length:12},(_,i)=>i+8);
  return (
    <>
      <B_Topbar title="Equipment" crumbs="Friday, Apr 19" actions={<><button className="btn btn--sm">‹</button><button className="btn btn--sm">Today</button><button className="btn btn--sm">›</button><button className="btn btn--primary"><Ic.Plus/>Book</button></>}/>
      <div className="sub-toolbar">
        <div className="filter-chip active">All</div><div className="filter-chip">My bookings</div><div className="filter-chip">Available now</div>
      </div>
      <div className="content">
        <div className="timeline-wrap">
          <div className="timeline-wrap__scroll">
            <div className="tline">
              <div className="tline-head">
                <div className="tline-head__left">Equipment</div>
                <div className="tline-hours">{hours.map(h=><div key={h} className="tline-hour">{String(h).padStart(2,'0')}:00</div>)}</div>
              </div>
              {EQUIPMENT.map(eq=>(
                <div className="tline-row" key={eq.id}>
                  <div className="tline-row__left"><Ic.Beaker style={{width:13,height:13,color:'var(--text-muted)'}}/>{eq.name}</div>
                  <div className="tline-lane">
                    {BOOKINGS.filter(b=>b.eq===eq.id).map((b,i)=>{
                      const left=(b.start-8)*72, w=(b.end-b.start)*72;
                      return <div key={i} className={'tline-block'+(b.own?' tline-block--own':'')} style={{left, width:w-2}}>
                        <strong style={{fontFamily:'JetBrains Mono,monospace',fontSize:11}}>{b.user}</strong><span style={{opacity:.7, fontSize:11, marginLeft:4}}>{b.purpose}</span>
                      </div>;
                    })}
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </>
  );
}

function B_Tokens() {
  return (
    <>
      <B_Topbar title="API Tokens"/>
      <div className="content">
        <div style={{display:'grid',gridTemplateColumns:'1fr 2fr',gap:16}}>
          <div style={{background:'var(--surface-default)',border:'1px solid var(--b-border)',borderRadius:8,padding:16}}>
            <div className="u-eyebrow" style={{marginBottom:12}}>Generate new token</div>
            <div className="field"><label>Name</label><input placeholder="e.g. Mobile app"/></div>
            <div className="field" style={{marginTop:10}}><label>Expiration</label><select><option>30 days</option><option>Never</option></select></div>
            <button className="btn btn--primary" style={{marginTop:12,width:'100%'}}>Generate</button>
          </div>
          <table className="data-table">
            <thead><tr><th>Name</th><th>Created</th><th>Last used</th><th>Status</th><th></th></tr></thead>
            <tbody>
              {TOKENS.map(t => (
                <tr key={t.name}>
                  <td><span className="name">{t.name}</span></td>
                  <td style={{fontFamily:'JetBrains Mono,monospace',fontSize:12,color:'var(--text-muted)'}}>{t.created}</td>
                  <td style={{fontFamily:'JetBrains Mono,monospace',fontSize:12,color:'var(--text-muted)'}}>{t.used}</td>
                  <td>{t.status==='active'?bStatusChip('active'):<span style={{color:'var(--text-muted)',fontSize:11.5}}>Revoked</span>}</td>
                  <td>{t.status==='active' && <button className="btn btn--sm">Revoke</button>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div style={{background:'var(--surface-default)',border:'1px solid var(--b-border)',borderRadius:8,padding:16,marginTop:16}}>
          <div className="u-eyebrow" style={{marginBottom:8}}>Usage</div>
          <pre style={{margin:0,fontSize:12.5,background:'var(--surface-muted)',padding:10,borderRadius:6,border:'1px solid var(--b-border)'}}>Authorization: Bearer &lt;your-token&gt;</pre>
        </div>
      </div>
    </>
  );
}

function B_Admin() {
  return (
    <>
      <B_Topbar title="Admin"/>
      <div className="content">
        <div style={{display:'grid', gridTemplateColumns:'1fr 2fr', gap:16, marginBottom:16}}>
          <div style={{background:'var(--surface-default)',border:'1px solid var(--b-border)',borderRadius:8}}>
            <div className="u-between" style={{padding:'10px 14px',borderBottom:'1px solid var(--b-border)'}}><strong style={{fontSize:13}}>Groups</strong><button className="btn btn--sm"><Ic.Plus/></button></div>
            {['Barbaros','Ahmet','NewGroup'].map(g=>(
              <div key={g} style={{padding:'10px 14px', borderBottom:'1px solid var(--b-border)', display:'flex', justifyContent:'space-between',fontSize:13}}>
                <div><strong>{g}</strong><div style={{fontSize:11.5,color:'var(--text-muted)',fontFamily:'JetBrains Mono,monospace'}}>{g==='Barbaros'?3:g==='Ahmet'?2:1} members</div></div>
                <button className="btn btn--ghost btn--icon btn--sm"><Ic.Trash/></button>
              </div>
            ))}
          </div>
          <div style={{background:'var(--surface-default)',border:'1px solid var(--b-border)',borderRadius:8}}>
            <div className="u-between" style={{padding:'10px 14px',borderBottom:'1px solid var(--b-border)'}}><strong style={{fontSize:13}}>Equipment</strong><button className="btn btn--sm"><Ic.Plus/>Add</button></div>
            <table className="data-table" style={{border:'none',borderRadius:0}}>
              <thead><tr><th>Name</th><th>Access</th><th>Status</th><th></th></tr></thead>
              <tbody>{EQUIPMENT.map(e=>(<tr key={e.id}><td><span className="name">{e.name}</span></td><td style={{fontSize:11.5,color:'var(--text-muted)',fontFamily:'JetBrains Mono,monospace'}}>all groups</td><td>{bStatusChip('active')}</td><td><button className="btn btn--sm">Usage</button></td></tr>))}</tbody>
            </table>
          </div>
        </div>
        <div style={{background:'var(--surface-default)',border:'1px solid var(--b-border)',borderRadius:8}}>
          <div className="u-between" style={{padding:'10px 14px',borderBottom:'1px solid var(--b-border)'}}><strong style={{fontSize:13}}>Users · {USERS.length}</strong><div className="search"><Ic.Search/><input placeholder="Search users"/></div></div>
          <table className="data-table" style={{border:'none',borderRadius:0}}>
            <thead><tr><th>User</th><th>Group</th><th>Status</th><th>Role</th><th></th></tr></thead>
            <tbody>{USERS.map(u=>(
              <tr key={u.user}>
                <td><span className="name">{u.user}</span></td>
                <td>{u.group}</td>
                <td>{bStatusChip(u.status==='approved'?'active':'pending')}</td>
                <td>{u.admin?<span style={{fontSize:11.5,padding:'2px 8px',borderRadius:4,background:'var(--b-accent-soft)',color:'var(--b-accent)',fontFamily:'JetBrains Mono,monospace'}}>admin</span>:'member'}</td>
                <td><button className="btn btn--sm"><Ic.Edit/></button></td>
              </tr>
            ))}</tbody>
          </table>
        </div>
      </div>
    </>
  );
}

function DirectionB({ screen, setScreen, activeSample, setActiveSample }) {
  return (
    <div className="dirB">
      <div className="app-shell">
        <B_Sidebar screen={screen} setScreen={setScreen}/>
        <div className="main">
          {screen==='dashboard' && <B_Dashboard setScreen={setScreen} setActiveSample={setActiveSample}/>}
          {screen==='samples' && <B_Samples setScreen={setScreen} setActiveSample={setActiveSample}/>}
          {screen==='detail' && <B_Detail sample={activeSample} setScreen={setScreen}/>}
          {screen==='new' && <B_New setScreen={setScreen}/>}
          {screen==='wiki' && <B_Wiki setScreen={setScreen}/>}
          {screen==='wikiArticle' && <B_WikiArticle setScreen={setScreen}/>}
          {screen==='booking' && <B_Booking/>}
          {screen==='tokens' && <B_Tokens/>}
          {screen==='admin' && <B_Admin/>}
        </div>
      </div>
    </div>
  );
}

Object.assign(window, { DirectionB });
