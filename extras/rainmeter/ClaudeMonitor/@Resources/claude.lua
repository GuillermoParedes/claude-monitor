-- Claude Monitor skin: parses state.txt (KEY=value) fetched by the [mRaw]
-- WebParser measure and updates variables, colors and charts.

local C = {}          -- colores (se leen de [Variables])
local state = {}      -- ultimo estado completo leido
local STALE_SECS = 15

local function color(name) return SKIN:GetVariable(name) end

function Initialize()
  local la = os.getenv('LOCALAPPDATA')
  if la then
    SKIN:Bang('!SetVariable', 'ExePath', la .. '\\Programs\\claude-monitor\\claude-monitor.exe')
  end
  for _, k in ipairs({ 'cAccent', 'cAccentDim', 'cGreen', 'cAmber', 'cRed', 'cBlue', 'cDim', 'cOff', 'cTrack' }) do
    C[k] = color(k)
  end
  chartW = tonumber(SKIN:GetVariable('ChartW')) or 240
  chartH = tonumber(SKIN:GetVariable('ChartH')) or 30
end

local function readState()
  local m = SKIN:GetMeasure('mRaw')
  if not m then return nil end
  local raw = m:GetStringValue()
  if not raw or raw == '' then return nil end
  local t = {}
  for line in raw:gmatch('[^\n]+') do
    local k, v = line:match('^([%w_]+)=(.*)$')
    if k then t[k] = (v:gsub('\r$', '')) end
  end
  if t.END ~= '1' then return nil end -- incomplete response: ignore
  return t
end

local function set(k, v) SKIN:Bang('!SetVariable', k, v == nil and '' or tostring(v)) end

local function split(s)
  local out = {}
  for x in string.gmatch(s or '', '([^,]+)') do out[#out + 1] = x end
  return out
end

local function levelColor(pct, warn, crit, base)
  if pct >= crit then return C.cRed end
  if pct >= warn then return C.cAmber end
  return base
end

local function fmtAgo(secs)
  if secs < 120 then return secs .. 's' end
  if secs < 7200 then return math.floor(secs / 60) .. 'm' end
  return math.floor(secs / 3600) .. 'h'
end

-- Barras verticales dentro de un meter Shape (Shape2..ShapeN).
local function bars(meter, values, highlight, gap)
  local n = #values
  local max = 0
  for i = 1, n do
    local v = tonumber(values[i]) or 0
    if v > max then max = v end
  end
  local slot = chartW / n
  local w = math.max(1, slot - gap)
  for i = 1, n do
    local v = tonumber(values[i]) or 0
    local h = (max > 0) and math.max(v > 0 and 2 or 1, math.floor(chartH * v / max + 0.5)) or 1
    local fill = (i == highlight) and C.cAccent or C.cAccentDim
    if v == 0 then fill = C.cTrack end
    SKIN:Bang('!SetOption', meter, 'Shape' .. (i + 1),
      string.format('Rectangle %.1f,%d,%.1f,%d,1 | Fill Color %s | StrokeWidth 0',
        (i - 1) * slot, chartH - h, w, h, fill))
  end
  return max
end

function Update()
  local t = readState()
  if t then state = t end
  local s = state
  if not s.UPDATED_TS then
    set('StatusText', 'waiting for data...')
    set('DotColor', C.cOff)
    SKIN:Bang('!UpdateMeter', '*')
    SKIN:Bang('!Redraw')
    return 0
  end

  for k, v in pairs(s) do set(k, v) end

  -- Estado general / datos rancios ----------------------------------------
  local age = os.time() - (tonumber(s.UPDATED_TS) or 0)
  local status = s.STATUS
  if age > STALE_SECS then
    set('StatusText', 'no data for ' .. fmtAgo(age))
    set('DotColor', C.cRed)
    set('StatusColor', C.cRed)
  else
    local map = {
      working = { 'working', C.cGreen },
      waiting = { 'needs permission', C.cAmber },
      idle    = { 'idle', C.cBlue },
      off     = { 'no sessions', C.cOff },
    }
    local m = map[status] or map.off
    set('StatusText', m[1])
    set('DotColor', m[2])
    set('StatusColor', status == 'waiting' and C.cAmber or C.cDim)
  end

  local more = tonumber(s.MORE) or 0
  set('MoreText', more > 0 and ('+' .. more .. ' more') or '')

  -- Limites del plan --------------------------------------------------------
  local p5, p7 = tonumber(s.LIM5_PCT) or -1, tonumber(s.LIM7_PCT) or -1
  if p5 < 0 and p7 < 0 then
    SKIN:Bang('!HideMeterGroup', 'Plan')
    SKIN:Bang('!ShowMeterGroup', 'PlanEmpty')
  else
    SKIN:Bang('!ShowMeterGroup', 'Plan')
    SKIN:Bang('!HideMeterGroup', 'PlanEmpty')
    set('L5', math.max(0, p5)); set('L7', math.max(0, p7))
    set('L5Color', levelColor(p5, 70, 90, C.cGreen))
    set('L7Color', levelColor(p7, 70, 90, C.cGreen))
    set('L5Text', p5 < 0 and '--' or (p5 .. '%' .. (s.LIM5_RESET ~= '' and ('  -  ' .. s.LIM5_RESET) or '')))
    set('L7Text', p7 < 0 and '--' or (p7 .. '%' .. (s.LIM7_RESET ~= '' and ('  -  ' .. s.LIM7_RESET) or '')))
  end

  -- Sesiones ----------------------------------------------------------------
  for i = 1, 4 do
    local p = 'S' .. i .. '_'
    if (s[p .. 'NAME'] or '') == '' then
      SKIN:Bang('!HideMeterGroup', 'S' .. i)
    else
      SKIN:Bang('!ShowMeterGroup', 'S' .. i)
      local ctx = tonumber(s[p .. 'CTX']) or 0
      set('c' .. i, levelColor(ctx, 75, 90, C.cAccent))
      local flag = s[p .. 'FLAG']
      set('sc' .. i, flag == 'waiting' and C.cAmber or (flag == 'working' and C.cGreen or C.cDim))
    end
  end
  set('NoSessions', (s.S1_NAME or '') == '' and 'no open sessions' or '')

  -- Graficos ----------------------------------------------------------------
  local hours = split(s.HOURS)
  if #hours == 24 then
    local maxH = bars('MeterHours', hours, (tonumber(s.HOUR_NOW) or 0) + 1, 2)
    set('HoursMax', string.format('peak $%.2f/h', maxH))
  end
  local days = split(s.DAYS)
  if #days == 7 then
    bars('MeterDays', days, 7, 6)
    local labels = split(s.DAY_LABELS)
    for i = 1, 7 do
      set('DL' .. i, labels[i] or '')
      set('DV' .. i, (tonumber(days[i]) or 0) >= 1 and string.format('%d', math.floor(tonumber(days[i]) + 0.5)) or '')
    end
  end

  -- Desglose ----------------------------------------------------------------
  local function top(prefix)
    local parts = {}
    for i = 1, 3 do
      local n = s[prefix .. i .. '_NAME'] or ''
      if n ~= '' then parts[#parts + 1] = n .. ' $' .. (s[prefix .. i .. '_COST'] or '0') end
    end
    return #parts > 0 and table.concat(parts, '   ') or '--'
  end
  set('TopProjects', top('P'))
  set('TopModels', top('M'))

  SKIN:Bang('!UpdateMeter', '*')
  SKIN:Bang('!Redraw')
  return 0
end
