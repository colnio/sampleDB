// Sample data for the lab redesign
const SAMPLES = [
  { id: 'SMP-042', name: 'GaAs Wafer Batch 042', createdAt: '2026-04-12', owner: 'j.chen', status: 'active', description: 'MBE-grown GaAs on Si substrate. Buffer layer 200 nm; verified by XRD. Device sizes 2–20 μm, 6 configs.', keywords: ['GaAs','MBE','heterostructure'], palette: 'blue', steps: 5, attachments: 8 },
  { id: 'SMP-D07', name: 'NV-Center Diamond D7', createdAt: '2026-04-09', owner: 'k.nakamura', status: 'progress', description: 'CVD diamond plate, nitrogen-implanted, annealed at 800 °C. PL spectrum confirms NV⁻ resonance.', keywords: ['diamond','NV','quantum'], palette: 'teal', steps: 7, attachments: 12 },
  { id: 'SMP-G115', name: 'Graphene Monolayer G-115', createdAt: '2026-04-07', owner: 'msmith', status: 'active', description: 'CVD on copper foil; transferred to SiO₂/Si. Raman 2D peak at 2680 cm⁻¹. Sheet resistance 510 Ω/□.', keywords: ['graphene','CVD','2D'], palette: 'violet', steps: 4, attachments: 6 },
  { id: 'SMP-N231',name: 'Superconducting Nb #231', createdAt: '2026-04-05', owner: 'r.ivanov', status: 'active', description: 'Sputter-deposited Nb, 150 nm thickness, Tc measured at 9.1 K. Residual resistivity ratio 24.', keywords: ['Nb','superconductor','film'], palette: 'emerald', steps: 6, attachments: 4 },
  { id: 'SMP-PS19',name: 'Perovskite Solar PS-19', createdAt: '2026-04-02', owner: 'a.patel', status: 'progress', description: 'MAPbI₃ spin-coated active layer. Encapsulated, tracked in glovebox. PCE 21.4% at AM1.5G.', keywords: ['perovskite','photovoltaic'], palette: 'amber', steps: 9, attachments: 15 },
  { id: 'SMP-H04', name: 'Si/SiGe Heterostructure H-04', createdAt: '2026-03-28', owner: 'l.okafor', status: 'archived', description: 'Strained Si quantum well on relaxed SiGe. Hall bar fabricated, mobility 210,000 cm²/Vs at 4 K.', keywords: ['Si','SiGe','quantum well'], palette: 'blue', steps: 8, attachments: 9 },
  { id: 'SMP-T11', name: 'TMDC MoS₂ Flake T-11', createdAt: '2026-03-22', owner: 'j.chen', status: 'active', description: 'Exfoliated MoS₂ monolayer on h-BN substrate. PL peak at 1.88 eV confirms direct bandgap.', keywords: ['MoS2','TMDC','monolayer'], palette: 'rose', steps: 3, attachments: 5 },
  { id: 'SMP-C22', name: 'CsPbBr₃ Nanocrystal C-22', createdAt: '2026-03-18', owner: 'a.patel', status: 'archived', description: 'Hot-injection synthesis. QY 78%, emission 518 nm. Stored in toluene under N₂.', keywords: ['CsPbBr3','nanocrystal'], palette: 'emerald', steps: 5, attachments: 3 },
];

const STEPS = [
  { title: 'Substrate clean (acetone / IPA / O₂ plasma)', date: '2026-04-12', by: 'j.chen', tool: 'Bench 3' },
  { title: 'CA2DM Optical Lithography — bottom Cr/Au contacts', date: '2026-04-12', by: 'j.chen', tool: 'Optical', notes: '~3 nm Cr / 67 nm Au, HHV Thermal evap.' },
  { title: 'MAC sputtering (UV or XeCl chamber, CA2DM)', date: '2026-04-13', by: 'j.chen', tool: 'Sputter B' },
  { title: 'Optical Lithography — top Cr/Au contacts', date: '2026-04-14', by: 'j.chen', tool: 'Optical' },
  { title: 'Probe-station I-V characterisation', date: '2026-04-15', by: 'msmith', tool: 'Probe Station', notes: 'Ran 12 devices, 9 yield' },
];

const ARTICLES = [
  { slug: 'cleanroom-safety', title: 'Cleanroom Safety Checklist', createdAt: 'Apr 14, 2026', author: 'msmith', tag: 'Safety', summary: 'Mandatory gowning, chemical hygiene, emergency procedures.' },
  { slug: 'mbe-startup', title: 'MBE Chamber Startup Procedure', createdAt: 'Apr 11, 2026', author: 'j.chen', tag: 'Equipment', summary: 'Cryopump regen, source cell temperatures, RHEED alignment.' },
  { slug: 'raman-quick', title: 'Raman Spectrometer — Quick Reference', createdAt: 'Apr 09, 2026', author: 'k.nakamura', tag: 'Equipment', summary: 'Laser selection, grating, standard baselines for Si and graphene.' },
  { slug: 'cryostat', title: 'Cryostat Transfer Protocol', createdAt: 'Apr 05, 2026', author: 'r.ivanov', tag: 'Protocols', summary: 'Safe He transfer, purge cycles, and loading the sample puck.' },
  { slug: 'labeling', title: 'Sample Labeling Conventions', createdAt: 'Mar 30, 2026', author: 'msmith', tag: 'Admin', summary: 'SMP-XYZ format, batch codes, owner initials.' },
  { slug: 'q2-meeting', title: 'Lab Meeting Rotation (Q2 2026)', createdAt: 'Mar 22, 2026', author: 'msmith', tag: 'Admin', summary: 'Who presents when — updated every quarter.' },
];

const EQUIPMENT = [
  { id: 1, name: 'AFM Dimension Icon' },
  { id: 2, name: 'Zeiss Merlin SEM' },
  { id: 3, name: 'Raman Spectrometer' },
  { id: 4, name: 'E-beam Evaporator' },
  { id: 5, name: 'Probe Station' },
  { id: 6, name: 'Keithley 6517B' },
];

const BOOKINGS = [
  { eq: 1, start: 9, end: 11.5, user: 'j.chen', purpose: 'Topology G-115' },
  { eq: 1, start: 14, end: 16, user: 'msmith', purpose: 'Follow-up 042', own: true },
  { eq: 2, start: 10, end: 13, user: 'a.patel', purpose: 'SEM PS-19 cross' },
  { eq: 3, start: 11, end: 12.25, user: 'k.nakamura', purpose: 'PL D7' },
  { eq: 3, start: 15, end: 18, user: 'msmith', purpose: 'Raman transfers', own: true },
  { eq: 4, start: 12, end: 17, user: 'r.ivanov', purpose: 'Nb #232' },
  { eq: 5, start: 13, end: 14.5, user: 'l.okafor', purpose: 'I-V H-04' },
  { eq: 6, start: 16, end: 17.5, user: 'j.chen', purpose: 'Leakage sweep' },
];

const ACTIVITY = [
  { who: 'j.chen',    verb: 'created sample',            what: 'SMP-042', when: '2m ago' },
  { who: 'msmith',    verb: 'booked',                     what: 'Raman Spectrometer, 15:00–18:00', when: '14m ago' },
  { who: 'a.patel',   verb: 'uploaded 3 attachments to',  what: 'SMP-PS19', when: '32m ago' },
  { who: 'k.nakamura',verb: 'added fabrication step to',  what: 'SMP-D07', when: '1h ago' },
  { who: 'r.ivanov',  verb: 'edited',                     what: 'SMP-N231', when: '2h ago' },
  { who: 'msmith',    verb: 'published wiki article',     what: 'Cleanroom Safety Checklist', when: '3h ago' },
  { who: 'l.okafor',  verb: 'archived',                   what: 'SMP-H04', when: '5h ago' },
];

const USERS = [
  { user: 'msmith', group: 'Barbaros', status: 'approved', admin: true },
  { user: 'j.chen', group: 'Ahmet', status: 'approved', admin: false },
  { user: 'a.patel', group: 'Barbaros', status: 'approved', admin: false },
  { user: 'k.nakamura', group: 'Barbaros', status: 'approved', admin: false },
  { user: 'r.ivanov', group: 'Ahmet', status: 'approved', admin: false },
  { user: 'l.okafor', group: 'NewGroup', status: 'pending', admin: false },
];

const TOKENS = [
  { name: 'skill_check', created: '2026-04-18', used: '2026-04-18', status: 'active' },
  { name: 'CodeX_skill', created: '2026-04-17', used: 'Never', status: 'revoked' },
  { name: 'mobile_app',  created: '2026-03-02', used: '2026-04-19', status: 'active' },
];

Object.assign(window, { SAMPLES, STEPS, ARTICLES, EQUIPMENT, BOOKINGS, ACTIVITY, USERS, TOKENS });
