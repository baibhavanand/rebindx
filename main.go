package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/miekg/dns"
)

// Session state for shifting modes
type sessionState struct {
	ID        string    `json:"id"`
	StartTime time.Time `json:"start_time"`
	Count     uint64    `json:"count"`
}

type logEntry struct {
	ID        int    `json:"id"`
	Timestamp string `json:"timestamp"`
	RemoteIP  string `json:"remote_ip"`
	Query     string `json:"query"`
	Response  string `json:"response"`
	Mode      string `json:"mode"`
	Session   string `json:"session"`
}

var (
	baseDomain    string
	dashboardPass string
	dashboardUser string
	dashboardPort int
	listenIP      string
	
	sessions   = make(map[string]*sessionState)
	sessionsMu sync.RWMutex
	logs       []logEntry
	logsMu     sync.Mutex
	logCounter int
	maxLogs    = 100
)

const dashboardTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>rebindx // dns.rebind.tool</title>
    <link href="https://fonts.googleapis.com/css2?family=Fira+Code:wght@400;600&family=Inter:wght@400;700&display=swap" rel="stylesheet">
    <style>
        :root {
            --bg: #0a0b0d;
            --surface: rgba(25, 27, 31, 0.7);
            --primary: #00ff9d;
            --secondary: #00e5ff;
            --error: #ff4560;
            --text-main: #e1e1e6;
            --text-dim: #9494a5;
            --border: rgba(255, 255, 255, 0.08);
            --glow: rgba(0, 255, 157, 0.15);
        }

        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'Inter', sans-serif;
            background: var(--bg);
            color: var(--text-main);
            line-height: 1.6;
            overflow-x: hidden;
            background-image: 
                radial-gradient(circle at 20% 30%, rgba(0, 255, 157, 0.03) 0%, transparent 40%),
                radial-gradient(circle at 80% 70%, rgba(0, 229, 255, 0.03) 0%, transparent 40%);
        }

        .container { max-width: 1300px; margin: 0 auto; padding: 40px 20px; }

        header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 50px;
            padding-bottom: 20px;
            border-bottom: 1px solid var(--border);
        }

        .logo {
            font-family: 'Fira Code', monospace;
            font-size: 1.8rem;
            font-weight: 700;
            letter-spacing: -1px;
            color: var(--primary);
            text-shadow: 0 0 20px var(--glow);
        }

        .logo span { color: var(--text-dim); }

        .github-link {
            display: flex;
            align-items: center;
            gap: 8px;
            color: var(--text-dim);
            text-decoration: none;
            font-family: 'Fira Code', monospace;
            font-size: 0.9rem;
            transition: all 0.3s ease;
            padding: 8px 12px;
            border-radius: 8px;
            border: 1px solid transparent;
        }

        .github-link:hover {
            color: var(--primary);
            background: rgba(0, 255, 157, 0.05);
            border-color: rgba(0, 255, 157, 0.2);
            box-shadow: 0 0 15px rgba(0, 255, 157, 0.1);
        }

        .grid {
            display: grid;
            grid-template-columns: 380px 1fr;
            gap: 30px;
        }

        .card {
            background: var(--surface);
            backdrop-filter: blur(12px);
            -webkit-backdrop-filter: blur(12px);
            border: 1px solid var(--border);
            border-radius: 16px;
            padding: 30px;
            box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
            position: relative;
            overflow: hidden;
        }

        .card::before {
            content: '';
            position: absolute;
            top: 0; left: 0; width: 100%; height: 2px;
            background: linear-gradient(90deg, transparent, var(--primary), transparent);
            opacity: 0.3;
        }

        h2 {
            font-family: 'Fira Code', monospace;
            font-size: 1.1rem;
            text-transform: uppercase;
            letter-spacing: 2px;
            color: var(--secondary);
            margin-bottom: 25px;
            display: flex;
            align-items: center;
            gap: 10px;
        }

        h2::before {
            content: '>';
            color: var(--primary);
        }

        .form-group { margin-bottom: 20px; position: relative; }
        label {
            display: block;
            font-size: 0.7rem;
            text-transform: uppercase;
            letter-spacing: 1.5px;
            color: var(--text-dim);
            margin-bottom: 8px;
            font-weight: 700;
        }

        input, select {
            width: 100%;
            height: 48px;
            background: rgba(0, 0, 0, 0.4);
            border: 1px solid var(--border);
            color: var(--text-main);
            padding: 0 16px;
            border-radius: 10px;
            font-family: 'Fira Code', monospace;
            font-size: 0.9rem;
            transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
            outline: none;
            resize: none;
            -webkit-appearance: none;
            appearance: none;
        }

        input:focus, select:focus {
            background: rgba(0, 0, 0, 0.6);
            border-color: var(--primary);
            box-shadow: 0 0 20px rgba(0, 255, 157, 0.1), inset 0 0 10px rgba(0, 255, 157, 0.05);
        }

        select {
            background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='24' height='24' viewBox='0 0 24 24' fill='none' stroke='%239494a5' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpolyline points='6 9 12 15 18 9'%3E%3C/polyline%3E%3C/svg%3E");
            background-repeat: no-repeat;
            background-position: right 16px center;
            background-size: 16px;
            padding-right: 40px !important;
        }

        .url-box {
            background: rgba(0, 0, 0, 0.5);
            padding: 20px;
            border-radius: 12px;
            border: 1px dashed var(--primary);
            margin-top: 10px;
            word-break: break-all;
            font-family: 'Fira Code', monospace;
            font-size: 0.85rem;
            color: var(--primary);
            cursor: pointer;
            position: relative;
            transition: transform 0.2s;
        }

        .url-box:hover { transform: translateY(-2px); }
        .url-box::after {
            content: 'COPY URL';
            position: absolute;
            top: -10px; right: 10px;
            background: var(--primary);
            color: var(--bg);
            font-size: 0.6rem;
            padding: 2px 8px;
            border-radius: 4px;
            font-weight: 700;
        }

        table { 
            width: 100%; 
            border-collapse: separate; 
            border-spacing: 0; 
            table-layout: auto;
        }
        th {
            text-align: left;
            padding: 15px;
            font-size: 0.75rem;
            text-transform: uppercase;
            letter-spacing: 1px;
            color: var(--text-dim);
            border-bottom: 1px solid var(--border);
        }
        td {
            padding: 15px;
            font-size: 0.9rem;
            border-bottom: 1px solid var(--border);
            color: var(--text-main);
            word-break: break-all;
            vertical-align: middle;
        }

        .mono { font-family: 'Fira Code', monospace; font-size: 0.85rem; }

        .btn {
            display: inline-flex;
            align-items: center;
            justify-content: center;
            padding: 8px 16px;
            border-radius: 6px;
            font-weight: 600;
            font-size: 0.8rem;
            cursor: pointer;
            transition: all 0.2s;
            border: none;
            text-decoration: none;
            gap: 8px;
        }

        .btn-sm { padding: 4px 10px; font-size: 0.75rem; }
        .btn-error { background: rgba(255, 69, 96, 0.1); color: var(--error); border: 1px solid rgba(255, 69, 96, 0.2); }
        .btn-error:hover { background: var(--error); color: white; }
        .btn-primary { background: var(--primary); color: var(--bg); }
        .btn-secondary { background: rgba(0, 229, 255, 0.1); color: var(--secondary); border: 1px solid rgba(0, 229, 255, 0.2); }
        .btn-secondary:hover { background: var(--secondary); color: var(--bg); }

        .badge {
            padding: 2px 8px;
            border-radius: 4px;
            font-size: 0.7rem;
            font-weight: 700;
            text-transform: uppercase;
            background: rgba(0, 229, 255, 0.1);
            color: var(--secondary);
            border: 1px solid rgba(0, 229, 255, 0.2);
        }

        .scroll-area { 
            max-height: 500px; 
            overflow: auto;
            border-radius: 8px;
            background: rgba(0, 0, 0, 0.2);
        }
        ::-webkit-scrollbar { width: 6px; }
        ::-webkit-scrollbar-track { background: transparent; }
        ::-webkit-scrollbar-thumb { background: var(--border); border-radius: 10px; }
        ::-webkit-scrollbar-thumb:hover { background: var(--text-dim); }

        @media (max-width: 1000px) {
            .grid { grid-template-columns: 1fr; }
        }

        @media (max-width: 600px) {
            .container { padding: 8px; }
            header { flex-direction: column; align-items: stretch; gap: 15px; margin-bottom: 25px; }
            .refresh-controls { width: 100%; justify-content: space-between; gap: 10px; flex-wrap: wrap; }
            .card { padding: 16px; border-radius: 12px; }
            h2 { font-size: 0.95rem; margin-bottom: 15px; }
            table { min-width: 500px; }
            td, th { padding: 10px 8px; font-size: 0.8rem; }
            .logo { font-size: 1.4rem; }
            .scroll-area { max-height: 400px; }
            .responsive-hide { display: none; }
        }

        .refresh-controls {
            display: flex;
            align-items: center;
            gap: 25px;
        }
        
        .auto-refresh-label {
            font-size: 0.75rem;
            text-transform: uppercase;
            letter-spacing: 1px;
            color: var(--text-dim);
            display: flex;
            align-items: center;
            gap: 10px;
            cursor: pointer;
            user-select: none;
            transition: color 0.2s;
        }

        .auto-refresh-label:hover { color: var(--text-main); }
        .auto-refresh-label input { display: none; }

        .toggle-track {
            width: 32px;
            height: 18px;
            background: rgba(255, 255, 255, 0.05);
            border: 1px solid var(--border);
            border-radius: 20px;
            position: relative;
            transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
            display: inline-block;
            vertical-align: middle;
        }

        .toggle-track::after {
            content: '';
            position: absolute;
            top: 3px; left: 3px;
            width: 10px; height: 10px;
            background: var(--text-dim);
            border-radius: 50%;
            transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
        }

        input:checked + .toggle-track {
            background: rgba(0, 255, 157, 0.1);
            border-color: var(--primary);
            box-shadow: 0 0 10px var(--glow);
        }

        input:checked + .toggle-track::after {
            left: 17px;
            background: var(--primary);
            box-shadow: 0 0 8px var(--primary);
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <div style="display: flex; align-items: center; gap: 20px;">
                <div class="logo">rebind<span>x</span></div>
                <a href="https://github.com/baibhavanand/rebindx" target="_blank" class="github-link">
                    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 19c-5 1.5-5-2.5-7-3m14 6v-3.87a3.37 3.37 0 0 0-.94-2.61c3.14-.35 6.44-1.54 6.44-7A5.44 5.44 0 0 0 20 4.77 5.07 5.07 0 0 0 19.91 1S18.73.65 16 2.48a13.38 13.38 0 0 0-7 0C6.27.65 5.09 1 5.09 1A5.07 5.07 0 0 0 5 4.77a5.44 5.44 0 0 0-1.5 3.78c0 5.42 3.3 6.61 6.44 7A3.37 3.37 0 0 0 9 18.13V22"></path></svg>
                    <span>GitHub</span>
                </a>
            </div>
            <div class="refresh-controls">
                <label class="auto-refresh-label">
                    <input type="checkbox" id="auto-refresh">
                    <div class="toggle-track"></div>
                    <span style="display: flex; align-items: center;">Auto-refresh</span>
                </label>
                <button class="btn btn-secondary" onclick="fetchData()" style="height: 36px;">
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M23 4v6h-6"></path><path d="M1 20v-6h6"></path><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"></path></svg>
                    REFRESH
                </button>
                <div style="font-size: 0.9rem; color: var(--text-dim); display: flex; align-items: center; font-family: 'Fira Code', monospace; line-height: 1;">
                    <code>:53 (UDP)</code>
                </div>
            </div>
        </header>

        <div class="grid">
            <aside>
                <div class="card">
                    <h2>URL Crafter</h2>
                    <div class="form-group">
                        <label>Record Type</label>
                        <select id="rtype" onchange="updateURL()">
                            <option value="a">A / AAAA (IP Rebinding)</option>
                            <option value="cname">CNAME (Domain Rebinding)</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label id="label1">Payload (IP or "auto")</label>
                        <input type="text" id="ip1" value="auto" placeholder="e.g. 1.2.3.4 or auto" oninput="updateURL()">
                    </div>
                    <div class="form-group">
                        <label id="label2">Target (Local IP)</label>
                        <input type="text" id="ip2" value="127.0.0.1" placeholder="e.g. 127.0.0.1" oninput="updateURL()">
                    </div>
                    <div class="form-group">
                        <label>Execution Mode</label>
                        <select id="mode" onchange="updateURL()">
                            <option value="r">Random Shifting (50/50)</option>
                            <option value="i">Interval Shift Once</option>
                            <option value="ir">Interval Rotate (Cycle)</option>
                            <option value="t">Threshold Shift Once</option>
                            <option value="tr">Threshold Rotate (Cycle)</option>
                            <option value="s">Sequential Rotate (Every Q)</option>
                            <option value="both">Multi-Answer (A + A)</option>
                        </select>
                    </div>
                    <div class="form-group" id="mode-val-group" style="display: none;">
                        <label id="mode-val-label">Value</label>
                        <input type="number" id="mode-val" value="30" min="1" oninput="updateURL()">
                    </div>
                    <div class="form-group">
                        <label>Cache TTL (Seconds)</label>
                        <input type="number" id="ttl" value="0" min="0" oninput="updateURL()">
                    </div>
                    <div class="form-group">
                        <label>Session Identity</label>
                        <input type="text" id="sid" value="research-1" oninput="updateURL()">
                    </div>
                    <div class="url-box" id="generated-url" onclick="copyToClipboard()"></div>
                </div>

                <div class="card" style="margin-top: 30px;">
                    <h2>Active Targets</h2>
                    <div class="scroll-area" style="max-height: 300px;">
                        <table id="sessions-table">
                            <thead>
                                <tr><th>Session ID</th><th>Queries</th><th>Action</th></tr>
                            </thead>
                            <tbody>
                                {{range .Sessions}}
                                <tr>
                                    <td class="mono">{{.ID}}</td>
                                    <td class="mono" style="color: var(--text-dim)">{{.Count}}</td>
                                    <td style="text-align: right;">
                                        <button class="btn btn-sm btn-error" onclick="resetSession('{{.ID}}')">KILL</button>
                                    </td>
                                </tr>
                                {{else}}
                                <tr><td colspan="3" style="color: var(--text-dim); font-size: 0.8rem; text-align: center; padding: 20px;">No active sessions</td></tr>
                                {{end}}
                            </tbody>
                        </table>
                    </div>
                </div>
            </aside>

            <main>
                <div class="card">
                    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 25px;">
                        <h2>Intercept Logs</h2>
                        <button class="btn btn-sm btn-error" onclick="clearLogs()">PURGE ALL</button>
                    </div>
                    <div class="scroll-area">
                        <table id="logs-table">
                            <colgroup>
                                <col style="width: 70px;">
                                <col class="responsive-hide" style="width: 120px;">
                                <col style="width: auto;">
                                <col style="width: auto;">
                                <col class="responsive-hide" style="width: 70px;">
                                <col style="width: 30px;">
                            </colgroup>
                            <thead>
                                <tr>
                                    <th>Time</th>
                                    <th class="responsive-hide">Requester</th>
                                    <th>Query</th>
                                    <th>Served</th>
                                    <th class="responsive-hide">Mode</th>
                                    <th></th>
                                </tr>
                            </thead>
                                <tbody>
                                    {{range .Logs}}
                                    <tr>
                                        <td class="mono" style="color: var(--text-dim)">{{.Timestamp}}</td>
                                        <td class="mono responsive-hide">{{.RemoteIP}}</td>
                                        <td class="mono">{{.Query}}</td>
                                        <td class="mono" style="color: var(--primary)">{{.Response}}</td>
                                        <td class="responsive-hide"><span class="badge">{{.Mode}}</span></td>
                                        <td style="text-align: right;">
                                            <button class="btn btn-sm btn-error" onclick="deleteLog({{.ID}})">×</button>
                                        </td>
                                    </tr>
                                    {{else}}
                                    <tr><td colspan="6" style="text-align: center; color: var(--text-dim); padding: 40px;">Waiting for incoming DNS traffic...</td></tr>
                                    {{end}}
                                </tbody>
                            </table>
                        </div>
                    </div>
                </div>
            </main>
        </div>
    </div>

    <script>
        const base = '{{.Domain}}';

        function processLabel(val, type) {
            if (val === 'auto') return 'auto';
            if (type === 'cname') {
                return val.replace(/\./g, '--');
            }
            // IP to Hex logic
            if (val.includes('.')) {
                const parts = val.split('.');
                if (parts.length !== 4) return val;
                let hex = '';
                for (let part of parts) {
                    const h = parseInt(part).toString(16).padStart(2, '0');
                    hex += h;
                }
                return hex.length === 8 ? hex : val;
            }
            if (val.includes(':')) {
                let clean = val.replace(/:/g, '');
                if (clean.length === 32) return clean;
                return val;
            }
            return val;
        }

        function updateURL() {
            const rtype = document.getElementById('rtype').value;
            const raw1 = document.getElementById('ip1').value || 'auto';
            const raw2 = document.getElementById('ip2').value || (rtype === 'cname' ? 'google.com' : '127.0.0.1');
            
            // Update labels based on rtype
            document.getElementById('label1').innerText = rtype === 'cname' ? 'Payload (Domain)' : 'Payload (IP or "auto")';
            document.getElementById('label2').innerText = rtype === 'cname' ? 'Target (Domain)' : 'Target (Local IP)';

            const label1 = processLabel(raw1, rtype);
            const label2 = processLabel(raw2, rtype);
            
            let mode = document.getElementById('mode').value;
            const modeValGroup = document.getElementById('mode-val-group');
            const modeValLabel = document.getElementById('mode-val-label');
            const modeVal = document.getElementById('mode-val');

            if (['i', 'ir', 't', 'tr'].includes(mode)) {
                modeValGroup.style.display = 'block';
                modeValLabel.innerText = ['i', 'ir'].includes(mode) ? 'Seconds' : 'Query Count';
                mode += modeVal.value || '1';
            } else {
                modeValGroup.style.display = 'none';
            }

            const ttl = document.getElementById('ttl').value || '0';
            const sid = document.getElementById('sid').value || 'research';
            document.getElementById('generated-url').innerText = label1 + '.' + label2 + '.' + mode + '.' + ttl + '.' + sid + '.' + base;
        }

        async function copyToClipboard() {
            const text = document.getElementById('generated-url').innerText;
            const box = document.getElementById('generated-url');
            const originalText = box.innerText;

            const success = async () => {
                box.innerText = 'COPIED TO CLIPBOARD!';
                box.style.borderColor = '#00e5ff';
                setTimeout(() => {
                    box.innerText = originalText;
                    box.style.borderColor = '#00ff9d';
                }, 1000);
            };

            if (navigator.clipboard && navigator.clipboard.writeText) {
                try {
                    await navigator.clipboard.writeText(text);
                    await success();
                    return;
                } catch (err) {}
            }

            // Fallback for non-HTTPS
            const textArea = document.createElement("textarea");
            textArea.value = text;
            textArea.style.position = "fixed";
            textArea.style.left = "-9999px";
            textArea.style.top = "0";
            document.body.appendChild(textArea);
            textArea.focus();
            textArea.select();
            try {
                document.execCommand('copy');
                await success();
            } catch (err) {}
            document.body.removeChild(textArea);
        }

        async function fetchData() {
            try {
                const response = await fetch('/api/data');
                if (response.status === 401) { window.location.reload(); return; }
                const data = await response.json();
                renderSessions(data.Sessions);
                renderLogs(data.Logs);
            } catch (err) {
                console.error('Fetch failed:', err);
            }
        }

        function renderSessions(sessions) {
            const tbody = document.querySelector('#sessions-table tbody');
            const keys = Object.keys(sessions);
            if (keys.length === 0) {
                tbody.innerHTML = '<tr><td colspan="3" style="color: var(--text-dim); font-size: 0.8rem; text-align: center; padding: 20px;">No active sessions</td></tr>';
                return;
            }
            tbody.innerHTML = keys.map(id => 
                '<tr>' +
                    '<td class="mono">' + id + '</td>' +
                    '<td class="mono" style="color: var(--text-dim)">' + sessions[id].count + '</td>' +
                    '<td style="text-align: right;">' +
                        '<button class="btn btn-sm btn-error" onclick="resetSession(\'' + id + '\')">KILL</button>' +
                    '</td>' +
                '</tr>'
            ).join('');
        }

        function renderLogs(logs) {
            const tbody = document.querySelector('#logs-table tbody');
            if (logs.length === 0) {
                tbody.innerHTML = '<tr><td colspan="6" style="text-align: center; color: var(--text-dim); padding: 40px;">Waiting for incoming DNS traffic...</td></tr>';
                return;
            }
            tbody.innerHTML = logs.map(log => 
                '<tr>' +
                    '<td class="mono" style="color: var(--text-dim)">' + log.timestamp + '</td>' +
                    '<td class="mono responsive-hide">' + log.remote_ip + '</td>' +
                    '<td class="mono">' + log.query + '</td>' +
                    '<td class="mono" style="color: var(--primary)">' + log.response + '</td>' +
                    '<td class="responsive-hide"><span class="badge">' + log.mode + '</span></td>' +
                    '<td style="text-align: right;">' +
                        '<button class="btn btn-sm btn-error" onclick="deleteLog(' + log.id + ')">×</button>' +
                    '</td>' +
                '</tr>'
            ).join('');
        }

        async function resetSession(id) {
            await fetch('/reset?id=' + encodeURIComponent(id), { method: 'POST' });
            fetchData();
        }

        async function clearLogs() {
            await fetch('/clear-logs', { method: 'POST' });
            fetchData();
        }

        async function deleteLog(id) {
            await fetch('/delete-log?id=' + id, { method: 'POST' });
            fetchData();
        }

        setInterval(() => {
            if (document.getElementById('auto-refresh').checked) {
                fetchData();
            }
        }, 2000);

        updateURL();
    </script>
</body>
</html>
`

func getSession(id string) *sessionState {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	if s, ok := sessions[id]; ok {
		return s
	}
	s := &sessionState{ID: id, StartTime: time.Now()}
	sessions[id] = s
	return s
}

func addLog(e logEntry) {
	logsMu.Lock()
	defer logsMu.Unlock()
	logCounter++
	e.ID = logCounter
	logs = append([]logEntry{e}, logs...)
	if len(logs) > maxLogs {
		logs = logs[:maxLogs]
	}
}

func parseIPLabel(label string, remoteIP net.Addr) (net.IP, bool) {
	if label == "auto" {
		host, _, _ := net.SplitHostPort(remoteIP.String())
		return net.ParseIP(host), true
	}
	// Try IPv4 hex
	if len(label) == 8 {
		if b, err := hex.DecodeString(label); err == nil {
			return net.IP(b), true
		}
	}
	// Try IPv6 hex
	if len(label) == 32 {
		if b, err := hex.DecodeString(label); err == nil {
			return net.IP(b), true
		}
	}
	return nil, false
}

func basicAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if dashboardPass == "" {
			next.ServeHTTP(w, r)
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != dashboardUser || pass != dashboardPass {
			w.Header().Set("WWW-Authenticate", `Basic realm="rebindx dashboard"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func handleDNS(w dns.ResponseWriter, r *dns.Msg) {
	msg := dns.Msg{}
	msg.SetReply(r)
	msg.Authoritative = true

	for _, q := range r.Question {
		name := strings.ToLower(q.Name)
		if !strings.HasSuffix(name, baseDomain) {
			continue
		}

		sub := strings.TrimSuffix(name, baseDomain)
		labels := strings.Split(strings.Trim(sub, "."), ".")

		if len(labels) < 2 {
			continue
		}

		if strings.HasPrefix(labels[0], "rc") {
			rcode, err := strconv.Atoi(strings.TrimPrefix(labels[0], "rc"))
			if err == nil {
				msg.Rcode = rcode
				w.WriteMsg(&msg)
				return
			}
		}

		mode := "r"
		if len(labels) >= 3 {
			mode = labels[2]
		}
		ttl := uint32(0)
		if len(labels) >= 4 {
			if t, err := strconv.ParseUint(labels[3], 10, 32); err == nil {
				ttl = uint32(t)
			}
		}
		sessionID := "default"
		if len(labels) >= 5 {
			sessionID = labels[4]
		}

		sess := getSession(sessionID)
		ip1, isIP1 := parseIPLabel(labels[0], w.RemoteAddr())
		ip2, isIP2 := parseIPLabel(labels[1], w.RemoteAddr())

		if !isIP1 || !isIP2 {
			handleCNAME(w, r, &msg, labels[0], labels[1], mode, ttl, sess)
			return
		}

		var chosen []net.IP
		switch {
		case mode == "both":
			chosen = []net.IP{ip1, ip2}
		case mode == "r":
			if rand.Intn(2) == 0 {
				chosen = []net.IP{ip1}
			} else {
				chosen = []net.IP{ip2}
			}
		case strings.HasPrefix(mode, "ir"):
			interval, _ := strconv.Atoi(strings.TrimPrefix(mode, "ir"))
			if interval <= 0 {
				interval = 1
			}
			if (int(time.Since(sess.StartTime).Seconds())/interval)%2 == 0 {
				chosen = []net.IP{ip1}
			} else {
				chosen = []net.IP{ip2}
			}
		case strings.HasPrefix(mode, "i"):
			interval, _ := strconv.Atoi(strings.TrimPrefix(mode, "i"))
			if time.Since(sess.StartTime) < time.Duration(interval)*time.Second {
				chosen = []net.IP{ip1}
			} else {
				chosen = []net.IP{ip2}
			}
		case strings.HasPrefix(mode, "tr"):
			threshold, _ := strconv.ParseUint(strings.TrimPrefix(mode, "tr"), 10, 64)
			if threshold <= 0 {
				threshold = 1
			}
			sessionsMu.Lock()
			if (sess.Count/threshold)%2 == 0 {
				chosen = []net.IP{ip1}
			} else {
				chosen = []net.IP{ip2}
			}
			sess.Count++
			sessionsMu.Unlock()
		case strings.HasPrefix(mode, "t"):
			threshold, _ := strconv.ParseUint(strings.TrimPrefix(mode, "t"), 10, 64)
			sessionsMu.Lock()
			if sess.Count < threshold {
				chosen = []net.IP{ip1}
			} else {
				chosen = []net.IP{ip2}
			}
			sess.Count++
			sessionsMu.Unlock()
		case mode == "s":
			sessionsMu.Lock()
			if sess.Count%2 == 0 {
				chosen = []net.IP{ip1}
			} else {
				chosen = []net.IP{ip2}
			}
			sess.Count++
			sessionsMu.Unlock()
		default:
			chosen = []net.IP{ip1}
		}

		respStr := ""
		for _, ip := range chosen {
			if ip.To4() != nil {
				msg.Answer = append(msg.Answer, &dns.A{
					Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl},
					A:   ip,
				})
			} else {
				msg.Answer = append(msg.Answer, &dns.AAAA{
					Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: ttl},
					AAAA:ip,
				})
			}
			respStr += ip.String() + " "
		}

		addLog(logEntry{
			Timestamp: time.Now().Format("15:04:05"),
			RemoteIP:  w.RemoteAddr().String(),
			Query:     q.Name,
			Response:  strings.TrimSpace(respStr),
			Mode:      mode,
			Session:   sessionID,
		})
	}

	if len(msg.Answer) > 0 {
		w.WriteMsg(&msg)
	}
}

func handleCNAME(w dns.ResponseWriter, r *dns.Msg, msg *dns.Msg, target1, target2, mode string, ttl uint32, sess *sessionState) {
	msg.Authoritative = true
	var chosen string
	
	switch {
	case mode == "s":
		sessionsMu.Lock()
		if sess.Count%2 == 0 {
			chosen = target1
		} else {
			chosen = target2
		}
		sess.Count++
		sessionsMu.Unlock()
	case mode == "r":
		if rand.Intn(2) == 0 {
			chosen = target1
		} else {
			chosen = target2
		}
	case strings.HasPrefix(mode, "ir"):
		interval, _ := strconv.Atoi(strings.TrimPrefix(mode, "ir"))
		if interval <= 0 {
			interval = 1
		}
		if (int(time.Since(sess.StartTime).Seconds())/interval)%2 == 0 {
			chosen = target1
		} else {
			chosen = target2
		}
	case strings.HasPrefix(mode, "i"):
		interval, _ := strconv.Atoi(strings.TrimPrefix(mode, "i"))
		if time.Since(sess.StartTime) < time.Duration(interval)*time.Second {
			chosen = target1
		} else {
			chosen = target2
		}
	case strings.HasPrefix(mode, "tr"):
		threshold, _ := strconv.ParseUint(strings.TrimPrefix(mode, "tr"), 10, 64)
		if threshold <= 0 {
			threshold = 1
		}
		sessionsMu.Lock()
		if (sess.Count/threshold)%2 == 0 {
			chosen = target1
		} else {
			chosen = target2
		}
		sess.Count++
		sessionsMu.Unlock()
	case strings.HasPrefix(mode, "t"):
		threshold, _ := strconv.ParseUint(strings.TrimPrefix(mode, "t"), 10, 64)
		sessionsMu.Lock()
		if sess.Count < threshold {
			chosen = target1
		} else {
			chosen = target2
		}
		sess.Count++
		sessionsMu.Unlock()
	default:
		chosen = target1
	}

	// Replace -- with . to allow full domains in labels
	if strings.Contains(chosen, "--") {
		chosen = strings.ReplaceAll(chosen, "--", ".")
	}

	if !strings.HasSuffix(chosen, ".") {
		chosen += "."
	}

	msg.Answer = append(msg.Answer, &dns.CNAME{
		Hdr: dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: ttl},
		Target: chosen,
	})
	addLog(logEntry{
		Timestamp: time.Now().Format("15:04:05"),
		RemoteIP:  w.RemoteAddr().String(),
		Query:     r.Question[0].Name,
		Response:  chosen,
		Mode:      mode,
		Session:   sess.ID,
	})
	w.WriteMsg(msg)
}

func startDashboard(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	
	tmpl := template.Must(template.New("dashboard").Parse(dashboardTemplate))

	mux := http.NewServeMux()
	mux.HandleFunc("/", basicAuth(func(w http.ResponseWriter, r *http.Request) {
		sessionsMu.RLock()
		defer sessionsMu.RUnlock()
		logsMu.Lock()
		defer logsMu.Unlock()
		
		data := struct {
			Domain   string
			Sessions map[string]*sessionState
			Logs     []logEntry
		}{baseDomain, sessions, logs}
		tmpl.Execute(w, data)
	}))

	mux.HandleFunc("/api/data", basicAuth(func(w http.ResponseWriter, r *http.Request) {
		sessionsMu.RLock()
		defer sessionsMu.RUnlock()
		logsMu.Lock()
		defer logsMu.Unlock()
		
		data := struct {
			Sessions map[string]*sessionState `json:"Sessions"`
			Logs     []logEntry               `json:"Logs"`
		}{sessions, logs}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	}))

	mux.HandleFunc("/reset", basicAuth(func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		sessionsMu.Lock()
		delete(sessions, id)
		sessionsMu.Unlock()
		if r.Method == "POST" {
			w.WriteHeader(http.StatusOK)
		} else {
			http.Redirect(w, r, "/", http.StatusSeeOther)
		}
	}))

	mux.HandleFunc("/clear-logs", basicAuth(func(w http.ResponseWriter, r *http.Request) {
		logsMu.Lock()
		logs = []logEntry{}
		logsMu.Unlock()
		if r.Method == "POST" {
			w.WriteHeader(http.StatusOK)
		} else {
			http.Redirect(w, r, "/", http.StatusSeeOther)
		}
	}))

	mux.HandleFunc("/delete-log", basicAuth(func(w http.ResponseWriter, r *http.Request) {
		idStr := r.URL.Query().Get("id")
		id, _ := strconv.Atoi(idStr)
		logsMu.Lock()
		for i, l := range logs {
			if l.ID == id {
				logs = append(logs[:i], logs[i+1:]...)
				break
			}
		}
		logsMu.Unlock()
		if r.Method == "POST" {
			w.WriteHeader(http.StatusOK)
		} else {
			http.Redirect(w, r, "/", http.StatusSeeOther)
		}
	}))

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", dashboardPort),
		Handler: mux,
	}

	go func() {
		fmt.Printf("Dashboard available at http://localhost:%d\n", dashboardPort)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %s", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
	fmt.Println("Dashboard server stopped")
}

func main() {
	var domainFlag string
	flag.StringVar(&domainFlag, "domain", "", "Base domain for rebinding (e.g., rebind.test)")
	flag.StringVar(&dashboardPass, "pass", "", "Dashboard password (Basic Auth)")
	flag.StringVar(&dashboardUser, "user", "admin", "Dashboard username (Basic Auth)")
	flag.IntVar(&dashboardPort, "port", 8080, "Dashboard HTTP port")
	flag.StringVar(&listenIP, "ip", "0.0.0.0", "DNS server listen IP")
	flag.Parse()

	if domainFlag == "" {
		if flag.NArg() > 0 {
			domainFlag = flag.Arg(0)
		} else {
			fmt.Println("Error: -domain flag or positional argument required")
			flag.Usage()
			os.Exit(1)
		}
	}

	baseDomain = domainFlag
	if !strings.HasSuffix(baseDomain, ".") {
		baseDomain += "."
	}

	rand.Seed(time.Now().UnixNano())
	dns.HandleFunc(".", handleDNS)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	wg.Add(1)
	go startDashboard(ctx, &wg)

	dnsServer := &dns.Server{Addr: listenIP + ":53", Net: "udp"}
	go func() {
		fmt.Printf("rebindx DNS server running on %s:53\n", listenIP)
		if err := dnsServer.ListenAndServe(); err != nil {
			fmt.Printf("DNS server failed: %s\n", err)
		}
	}()

	<-ctx.Done()
	fmt.Println("\nShutting down...")
	dnsServer.Shutdown()
	wg.Wait()
	fmt.Println("rebindx exited cleanly")
}
