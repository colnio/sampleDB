## What this example demonstrates
This lab shows how **CodeX** can take a *messy, barely structured* data directory and still:
- infer **project context** from folder names and file naming conventions,
- infer **measurement types** (e.g., voltage/current, fluorescence intensity, absorbance, temperature, microscopy channel, etc.) from filenames + patterns,
- build a **clean dataset** (tidy tables + metadata),
- generate a reproducible **analysis notebook** (`.ipynb`) that follows analysis patterns commonly seen in research papers (QC → normalization → stats → figures → export).

> **Outcome:** a notebook and an `analysis/` folder containing figures + summary tables, produced from “almost no schema”.

---

## Starting point: intentionally messy dataset
You provide CodeX a directory that looks like this (example):
![alt text](directory_structure.png)

**Important characteristics (deliberately “barely structured”):**
- mixed formats (`log/data/png`)
- inconsistent naming (`qc2` vs `gr_mobility`)
- metadata hidden in filenames (`ctrl`, `treatA`, `rep1`, `ch1`)
- calibration files (`air`) mixed into the same tree
- duplicates and style collisions
- over 4500 unque data files
---

## The task for CodeX
Ask CodeX to:
1. **Scan** the directory and describe what it thinks the dataset contains.
2. Propose a **schema** (for example, split the data into projects, runs, samples, conditions, replicates, channels).
3. Produce a **parsing + ingestion plan** (what it will read, what it will ignore, how it will detect duplicates).
4. Generate an **analysis notebook** that:
   - loads the parsed data,
   - runs quality checks,
   - performs transformations (normalization / baseline correction / background subtraction),
   - computes summary statistics,
   - creates paper-style figures,
   - exports results (tables + plots).

![alt text](sorting_summary.png)

---

## Suggested prompt to give CodeX (copy/paste)
Use something like:

> “You are given a directory of experimental data with weak structure.  
> Please:  
> 1) scan and summarize the directory structure;  
> 2) infer projects/experiments and measurement types from file and folder naming;  
> 3) propose a metadata schema;  
> 4) implement a robust parser that creates tidy tables + extracted metadata;  
> 5) generate an `analysis.ipynb` that reproduces a paper-style analysis workflow (QC → normalization → statistics → figures);  
> 6) export `results/summary.csv` and `figures/*.png`.  
> Be conservative: log assumptions, handle duplicates, and create a `summary.md` file with condensed info about the sorting process.

---

## What CodeX should infer (examples of “reasonable guesses”)
CodeX should be able to infer things like:
- **Project identity** from top-level folders (`projectA`, `proj_B_misc`)
- **Experiment/run structure** from dated subfolders (`2025-11-02_run3`)
- **Conditions** from tokens (`ctrl`, `treatA`)
- **Replicates** from tokens (`rep1`, `rep2`)
- **Channels** from tokens (`ch1`, `GFP`, `RFP`)
- **Measurement type** from file types + naming:
  - `*_ch1.csv` might be time-series intensity or sensor readout
  - `cal_curve_*.csv` suggests calibration curves
  - `*_tempSweep_*.tsv` suggests temperature ramp experiments
  - `*_GFP_*.tif` suggests microscopy fluorescence images

---

## Notebook structure (what “good” looks like)
A strong `analysis.ipynb` output typically contains:

### 1) Directory scan + data catalog
- create a `data_catalog.csv`:
  - filepath
  - filetype
  - inferred project
  - inferred measurement type
  - inferred sample/condition/replicate/channel
  - any warnings (duplicate, missing tokens, unreadable)
  
![alt text](analysis_notebook.png)

### 2) Parsing + tidy dataset creation
- parse all tabular files into a unified long-form table, e.g.:

| project | run | sample | condition | replicate | channel | time | value | units |
|---|---|---|---|---|---|---:|---:|---|

- parse image metadata into a table:

| project | plate | well | marker | magnification | filepath |
|---|---|---|---|---|---|


### 3) Quality control (QC)
Typical paper-like QC steps:
- missing values, outliers, impossible ranges
- replicate consistency checks
- channel cross-checks (if relevant)
- detection of suspicious duplicates

![alt text](mobility_summary.png)

### 4) Normalization / preprocessing
Pick transformations appropriate to inferred measurement type:
- baseline subtraction (time-series sensors)
- z-score or min–max normalization
- blank/background correction (plate reader)
- calibration curve application if calibration files exist



### 5) Statistical analysis
Depending on inferred conditions:
- compare `ctrl` vs `treatA` (t-test / Mann–Whitney / ANOVA)
- effect size + confidence intervals
- multiple testing correction if many comparisons


### 6) Figures and outputs
- export:
  - `results/summary.csv` (aggregated metrics)
  - `results/qc_report.md` (assumptions + warnings)
  - `figures/figure_1.png`, `figure_2.png`, etc.

![alt text](stats.png)

---

## “Following examples from papers” (how to frame it)
To keep this lab broadly applicable across domains, phrase it as:

**Paper-inspired workflow pattern:**
1. Describe dataset + sample counts (like Methods)
2. QC and exclusions (like Supplementary Fig. S1)
3. Normalization rationale (like Methods)
4. Primary comparison plot (like Fig. 1)
5. Secondary analyses / sensitivity checks (like Fig. 2 / Fig. 3)
6. Export reproducible artifacts (like “Data & Code Availability”)

---

## Success criteria for the lab
At the end, learners should have:
- a `data_catalog.csv` describing all files + inferred metadata
- a `processed/` dataset (tidy tables)
- an `analysis.ipynb` that runs end-to-end without manual edits
- exported results and figures
- a clear log of assumptions and ambiguous cases

---

## Optional “hard mode” additions (if you want to push CodeX)
- introduce typos and inconsistencies: `tretA`, `contorl`, mixed delimiters
- include a few corrupted or empty files and expect graceful skipping
- include a second, subtly different experiment layout to test generalization
- include a “paper template” figure target (e.g., “make a 2x2 figure grid with panels A–D”)