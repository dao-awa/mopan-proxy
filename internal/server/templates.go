package server

var indexHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>MopanProxy - 和彩云加密代理</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#0f0f0f;color:#e0e0e0;min-height:100vh}
a{color:#4fc3f7;text-decoration:none}
a:hover{text-decoration:underline}
.btn{padding:8px 20px;border:none;border-radius:8px;cursor:pointer;font-size:14px;font-weight:500;transition:all .2s}
.btn:hover{transform:translateY(-1px)}
.btn-primary{background:#1a73e8;color:#fff}.btn-primary:hover{background:#1557b0}
.btn-success{background:#34a853;color:#fff}.btn-success:hover{background:#2d8e47}
.btn-danger{background:#ea4335;color:#fff}.btn-danger:hover{background:#c5221f}
.btn-ghost{background:transparent;color:#aaa;border:1px solid #333}.btn-ghost:hover{background:#222;color:#fff}
.btn-sm{padding:5px 12px;font-size:12px}
input,textarea{background:#1a1a1a;border:1px solid #333;border-radius:8px;padding:10px 14px;color:#e0e0e0;font-size:14px;width:100%;outline:none;transition:border .2s}
input:focus,textarea:focus{border-color:#1a73e8}
.container{max-width:1000px;margin:0 auto;padding:20px}

/* Header */
.header{display:flex;align-items:center;justify-content:space-between;padding:16px 24px;border-bottom:1px solid #222;background:#161616;position:sticky;top:0;z-index:100}
.header h1{font-size:20px;font-weight:600;color:#fff}
.header h1 span{color:#1a73e8}
.header-right{display:flex;align-items:center;gap:12px}
.badge{padding:4px 12px;border-radius:20px;font-size:12px;font-weight:500}
.badge-ok{background:#1a3a1a;color:#4caf50;border:1px solid #2d5a2d}
.badge-err{background:#3a1a1a;color:#f44336;border:1px solid #5a2d2d}
.nav-btn{padding:6px 14px;border-radius:6px;cursor:pointer;font-size:13px;color:#aaa;background:transparent;border:1px solid #333;transition:all .2s}
.nav-btn:hover,.nav-btn.active{background:#1a73e8;color:#fff;border-color:#1a73e8}

/* Login */
.login-panel{max-width:500px;margin:80px auto;background:#1a1a1a;border-radius:16px;padding:40px;border:1px solid #222}
.login-panel h2{font-size:24px;margin-bottom:8px;color:#fff}
.login-panel .subtitle{color:#888;font-size:14px;margin-bottom:24px;line-height:1.6}
.login-panel .step{background:#111;border-radius:8px;padding:12px 16px;margin-bottom:16px;font-size:13px;color:#aaa;line-height:1.8}
.login-panel .step code{background:#222;padding:2px 6px;border-radius:4px;color:#4fc3f7;font-size:12px}
.login-panel .step b{color:#e0e0e0}
.form-group{margin-bottom:16px}
.form-group label{display:block;font-size:13px;color:#888;margin-bottom:6px}
.form-hint{font-size:12px;color:#666;margin-top:4px}
.msg{padding:10px 14px;border-radius:8px;font-size:13px;margin-bottom:16px}
.msg-ok{background:#1a3a1a;color:#4caf50;border:1px solid #2d5a2d}
.msg-err{background:#3a1a1a;color:#f44336;border:1px solid #5a2d2d}

/* File list */
.toolbar{display:flex;align-items:center;justify-content:space-between;margin-bottom:16px;gap:12px;flex-wrap:wrap}
.breadcrumb{display:flex;align-items:center;gap:4px;font-size:13px;color:#888;flex-wrap:wrap}
.breadcrumb span{cursor:pointer;padding:4px 8px;border-radius:4px;transition:background .2s}
.breadcrumb span:hover{background:#222;color:#fff}
.breadcrumb span.current{color:#fff;font-weight:500}
table{width:100%;border-collapse:collapse}
th{text-align:left;padding:10px 12px;font-size:12px;color:#888;font-weight:500;border-bottom:1px solid #222;text-transform:uppercase;letter-spacing:.5px}
td{padding:10px 12px;border-bottom:1px solid #1a1a1a;font-size:14px}
tr:hover td{background:#1a1a1a}
tr.folder td{cursor:pointer}
tr.folder td:first-child{color:#4fc3f7;font-weight:500}
.file-icon{display:inline-block;width:20px;text-align:center;margin-right:8px}
.empty{text-align:center;padding:60px 20px;color:#666;font-size:15px}

/* Upload */
.upload-zone{border:2px dashed #333;border-radius:12px;padding:40px;text-align:center;cursor:pointer;transition:border .2s}
.upload-zone:hover,.upload-zone.dragover{border-color:#1a73e8;background:#0d1a2d}
.upload-zone p{color:#888;font-size:14px;margin-top:8px}

/* Settings */
.settings-section{background:#1a1a1a;border-radius:12px;padding:24px;margin-bottom:20px;border:1px solid #222}
.settings-section h3{font-size:16px;margin-bottom:16px;color:#fff}
.settings-row{display:flex;align-items:center;gap:12px;margin-bottom:12px}
.settings-row label{min-width:120px;font-size:13px;color:#888}
.settings-row input{max-width:300px}

/* Loading */
.loading{display:inline-block;width:20px;height:20px;border:2px solid #333;border-top-color:#1a73e8;border-radius:50%;animation:spin .6s linear infinite}
@keyframes spin{to{transform:rotate(360deg)}}

.hidden{display:none!important}
</style>
</head>
<body>

<div class="header">
  <h1>&#128274; <span>Mopan</span>Proxy</h1>
  <div class="header-right">
    <div id="statusBadge" class="badge badge-err">未连接</div>
    <button class="nav-btn" id="navFiles" data-page="files" style="display:none">&#128193; 文件</button>
    <button class="nav-btn" id="navSettings" data-page="settings" style="display:none">&#9881; 设置</button>
    <button class="nav-btn" id="navLogout" data-action="logout" style="display:none;color:#f44336">退出</button>
  </div>
</div>

<div class="container">
  <!-- Login -->
  <div id="loginPanel" class="login-panel">
    <h2>&#128273; 连接和彩云</h2>
    <p class="subtitle">通过浏览器提取 Token 来连接你的和彩云账号</p>
    <div id="loginMsg" class="msg hidden"></div>
    <div class="step">
      <b>步骤：</b><br>
      1. 打开 <a href="https://yun.139.com/w/" target="_blank">yun.139.com/w/</a> 并登录<br>
      2. 按 <code>F12</code> 打开开发者工具<br>
      3. 切到 <code>Network</code> 标签，刷新页面<br>
      4. 找任意请求，复制 <code>Authorization</code> 头的值
    </div>
    <div class="form-group">
      <label>Authorization Token</label>
      <input id="tokenInput" type="text" placeholder="Basic cGM6MTk4..." style="font-family:monospace;font-size:12px">
      <p class="form-hint">格式: Basic base64(pc:手机号:token)</p>
    </div>
    <button class="btn btn-primary" style="width:100%" id="btnConnect">连接</button>
  </div>

  <!-- Files -->
  <div id="filesPanel" class="hidden">
    <div class="toolbar">
      <div id="breadcrumb" class="breadcrumb"></div>
      <div style="display:flex;gap:8px">
        <button class="btn btn-primary btn-sm" id="btnUpload">上传</button>
        <button class="btn btn-ghost btn-sm" id="btnMkdir">新建文件夹</button>
      </div>
    </div>
    <table>
      <thead><tr><th>名称</th><th style="width:100px">大小</th><th style="width:160px">修改时间</th><th style="width:60px">操作</th></tr></thead>
      <tbody id="fileList"></tbody>
    </table>
    <div id="emptyMsg" class="empty hidden">此目录为空</div>
    <div id="loading" class="hidden" style="text-align:center;padding:40px"><div class="loading"></div></div>
  </div>

  <!-- Upload -->
  <div id="uploadPanel" class="hidden">
    <div class="toolbar">
      <h3>上传文件</h3>
      <button class="btn btn-ghost btn-sm" id="btnBackFiles">返回</button>
    </div>
    <div class="upload-zone" id="dropZone">
      <p>&#128193; 拖拽文件到这里，或点击选择</p>
      <input type="file" id="fileInput" multiple style="display:none">
    </div>
    <div id="uploadProgress" style="margin-top:16px"></div>
  </div>

  <!-- Settings -->
  <div id="settingsPanel" class="hidden">
    <div class="toolbar">
      <h3 style="font-size:18px">设置</h3>
      <button class="btn btn-ghost btn-sm" id="btnBackFiles2">返回</button>
    </div>
    <div class="settings-section">
      <h3>&#128274; 和彩云 Token</h3>
      <div class="form-group">
        <label>当前账号</label>
        <input id="settAccount" type="text" readonly style="opacity:.6">
      </div>
      <div class="form-group">
        <label>更新 Token</label>
        <input id="settToken" type="text" placeholder="粘贴新的 Authorization Token" style="font-family:monospace;font-size:12px">
      </div>
      <button class="btn btn-primary btn-sm" id="btnUpdateToken">更新 Token</button>
    </div>
    <div class="settings-section">
      <h3>&#128272; WebDAV 设置</h3>
      <div class="form-group">
        <label>端口</label>
        <input id="settWebdavPort" type="number" style="max-width:120px">
      </div>
      <div class="form-group">
        <label>用户名</label>
        <input id="settWebdavUser" type="text">
      </div>
      <div class="form-group">
        <label>密码</label>
        <input id="settWebdavPass" type="text">
      </div>
      <button class="btn btn-primary btn-sm" id="btnUpdateWebdav">保存 WebDAV 设置</button>
    </div>
    <div class="settings-section">
      <h3>&#128196; 系统信息</h3>
      <p style="color:#888;font-size:13px">
        WebDAV 地址: <code id="webdavUrl" style="background:#222;padding:2px 8px;border-radius:4px;color:#4fc3f7"></code>
      </p>
    </div>
  </div>
</div>

<script>
var currentPath = '/';
var currentFiles = [];

function apiPost(url, data, callback) {
  var xhr = new XMLHttpRequest();
  xhr.open('POST', url);
  xhr.setRequestHeader('Content-Type', 'application/json');
  xhr.setRequestHeader('X-Requested-With', 'XMLHttpRequest');
  xhr.onload = function() {
    try {
      var d = JSON.parse(xhr.responseText);
      callback(d);
    } catch(e) {
      callback({success: false, error: 'parse error'});
    }
  };
  xhr.onerror = function() { callback({success: false, error: 'network error'}); };
  xhr.send(data ? JSON.stringify(data) : null);
}

function apiGet(url, callback) {
  var xhr = new XMLHttpRequest();
  xhr.open('GET', url);
  xhr.setRequestHeader('X-Requested-With', 'XMLHttpRequest');
  xhr.onload = function() {
    try {
      var d = JSON.parse(xhr.responseText);
      callback(d);
    } catch(e) {
      callback({success: false, error: 'parse error'});
    }
  };
  xhr.onerror = function() { callback({success: false, error: 'network error'}); };
  xhr.send();
}

function showPage(page) {
  document.getElementById('loginPanel').classList.add('hidden');
  document.getElementById('filesPanel').classList.add('hidden');
  document.getElementById('uploadPanel').classList.add('hidden');
  document.getElementById('settingsPanel').classList.add('hidden');
  document.querySelectorAll('.nav-btn').forEach(function(b){b.classList.remove('active')});

  if (page === 'files') {
    document.getElementById('filesPanel').classList.remove('hidden');
    document.getElementById('navFiles').classList.add('active');
    loadFiles(currentPath);
  } else if (page === 'settings') {
    document.getElementById('settingsPanel').classList.remove('hidden');
    document.getElementById('navSettings').classList.add('active');
    loadSettings();
  } else if (page === 'upload') {
    document.getElementById('uploadPanel').classList.remove('hidden');
  }
}

function checkStatus() {
  apiGet('/api/status', function(d) {
    var b = document.getElementById('statusBadge');
    if (d.data && d.data.auth) {
      b.textContent = '✓ ' + d.data.account;
      b.className = 'badge badge-ok';
      document.getElementById('loginPanel').classList.add('hidden');
      document.getElementById('navFiles').style.display = '';
      document.getElementById('navSettings').style.display = '';
      document.getElementById('navLogout').style.display = '';
      showPage('files');
    } else {
      b.textContent = '未连接';
      b.className = 'badge badge-err';
      document.getElementById('loginPanel').classList.remove('hidden');
      document.getElementById('navFiles').style.display = 'none';
      document.getElementById('navSettings').style.display = 'none';
      document.getElementById('navLogout').style.display = 'none';
    }
  });
}

function setToken() {
  var token = document.getElementById('tokenInput').value.trim();
  if (!token) return showLoginMsg('请输入 Token', false);
  apiPost('/api/set-token', {authorization: token}, function(d) {
    if (d.success) {
      showLoginMsg('连接成功: ' + d.data.account, true);
      setTimeout(checkStatus, 300);
    } else {
      showLoginMsg(d.error || '连接失败', false);
    }
  });
}

function loadFiles(path) {
  document.getElementById('loading').classList.remove('hidden');
  document.getElementById('fileList').innerHTML = '';
  document.getElementById('emptyMsg').classList.add('hidden');
  currentPath = path;

  apiGet('/api/files?path=' + encodeURIComponent(path), function(d) {
    document.getElementById('loading').classList.add('hidden');
    if (!d.success) {
      document.getElementById('emptyMsg').textContent = d.error || '加载失败';
      document.getElementById('emptyMsg').classList.remove('hidden');
      return;
    }
    currentFiles = d.data.files || [];
    renderBreadcrumb(path);
    if (currentFiles.length === 0) {
      document.getElementById('emptyMsg').classList.remove('hidden');
    } else {
      renderFiles(currentFiles);
    }
  });
}

function renderBreadcrumb(path) {
  var bc = document.getElementById('breadcrumb');
  var parts = path.split('/').filter(function(p){return p});
  var html = '<span data-path="/" class="' + (parts.length===0?'current':'') + '">根目录</span>';
  var accum = '';
  parts.forEach(function(p, i) {
    accum += '/' + p;
    var isLast = i === parts.length - 1;
    html += ' / <span data-path="' + escAttr(accum) + '" class="' + (isLast?'current':'') + '">' + escHtml(p) + '</span>';
  });
  bc.innerHTML = html;
}

function renderFiles(files) {
  var tbody = document.getElementById('fileList');
  files.sort(function(a,b){ return a.type === b.type ? 0 : (a.type==='folder'?-1:1); });
  var html = '';
  files.forEach(function(f) {
    var icon = f.type === 'folder' ? '&#128193;' : getFileIcon(f.name);
    var size = f.type === 'folder' ? '-' : formatSize(f.size);
    var date = f.updated_at ? f.updated_at.substring(0, 16).replace('T',' ') : '-';
    var filePath = (currentPath === '/' ? '/' : currentPath + '/') + f.name;
    html += '<tr class="' + f.type + '" data-path="' + escAttr(filePath) + '">';
    html += '<td><span class="file-icon">' + icon + '</span>' + escHtml(f.name) + '</td>';
    html += '<td style="color:#888">' + size + '</td>';
    html += '<td style="color:#888">' + date + '</td>';
    html += '<td>';
    if (f.type === 'file') html += '<a href="/api/download/' + escAttr(f.id) + '" class="btn btn-ghost btn-sm">下载</a> ';
    html += '<button class="btn btn-ghost btn-sm btn-delete" data-id="' + escAttr(f.id) + '" data-name="' + escAttr(f.name) + '">删除</button>';
    html += '</td></tr>';
  });
  tbody.innerHTML = html;
}

function deleteFile(id, name) {
  if (!confirm('确定删除 "' + name + '"？')) return;
  apiPost('/api/delete', {file_ids: [id]}, function(d) {
    if (d.success) loadFiles(currentPath);
    else alert(d.error || '删除失败');
  });
}

function showUpload() { showPage('upload'); }

function showMkdir() {
  var name = prompt('新建文件夹名称:');
  if (!name) return;
  apiPost('/api/mkdir', {parent_id: currentPath, name: name}, function(d) {
    if (d.success) loadFiles(currentPath);
    else alert(d.error || '创建失败');
  });
}

function loadSettings() {
  apiGet('/api/settings', function(d) {
    if (!d.success) return;
    var s = d.data;
    document.getElementById('settAccount').value = s.account || '';
    document.getElementById('settWebdavPort').value = s.webdav_port || 9091;
    document.getElementById('settWebdavUser').value = s.webdav_user || '';
    document.getElementById('settWebdavPass').value = s.webdav_pass || '';
    document.getElementById('webdavUrl').textContent = location.protocol + '//' + location.hostname + ':' + (s.webdav_port||9091) + '/';
  });
}

function updateToken() {
  var token = document.getElementById('settToken').value.trim();
  if (!token) return;
  apiPost('/api/set-token', {authorization: token}, function(d) {
    if (d.success) { alert('Token 已更新'); loadSettings(); }
    else alert(d.error || '更新失败');
  });
}

function updateWebdav() {
  apiPost('/api/settings/update', {
    webdav_port: parseInt(document.getElementById('settWebdavPort').value),
    webdav_user: document.getElementById('settWebdavUser').value,
    webdav_pass: document.getElementById('settWebdavPass').value
  }, function(d) {
    if (d.success) alert('设置已保存');
    else alert(d.error || '保存失败');
  });
}

function logout() {
  apiPost('/api/set-token', {authorization: ''}, function(){checkStatus()});
}

// Event delegation — H1 XSS 修复：所有交互通过 data-* 属性 + 事件委托
document.addEventListener('click', function(e) {
  var target = e.target;

  // 导航按钮
  if (target.dataset.page) {
    showPage(target.dataset.page);
    return;
  }

  // 退出按钮
  if (target.dataset.action === 'logout') {
    logout();
    return;
  }

  // 面包屑导航
  if (target.closest('.breadcrumb') && target.dataset.path) {
    loadFiles(target.dataset.path);
    return;
  }

  // 文件夹行点击
  var folderTr = target.closest('tr.folder');
  if (folderTr && folderTr.dataset.path && !target.closest('.btn-delete') && !target.closest('a')) {
    loadFiles(folderTr.dataset.path);
    return;
  }

  // 删除按钮
  if (target.classList.contains('btn-delete')) {
    deleteFile(target.dataset.id, target.dataset.name);
    return;
  }

  // 上传按钮
  if (target.id === 'btnUpload' || target.id === 'btnBackFiles' || target.id === 'btnBackFiles2') {
    showPage(target.id === 'btnUpload' ? 'upload' : 'files');
    return;
  }

  // 新建文件夹按钮
  if (target.id === 'btnMkdir') {
    showMkdir();
    return;
  }

  // 连接按钮
  if (target.id === 'btnConnect') {
    setToken();
    return;
  }

  // 更新 Token 按钮
  if (target.id === 'btnUpdateToken') {
    updateToken();
    return;
  }

  // 更新 WebDAV 按钮
  if (target.id === 'btnUpdateWebdav') {
    updateWebdav();
    return;
  }

  // 上传区域点击
  if (target.id === 'dropZone' || target.closest('#dropZone')) {
    document.getElementById('fileInput').click();
    return;
  }
});

// Upload handling
var dropZone = document.getElementById('dropZone');
if (dropZone) {
  dropZone.addEventListener('dragover', function(e){e.preventDefault();this.classList.add('dragover')});
  dropZone.addEventListener('dragleave', function(){this.classList.remove('dragover')});
  dropZone.addEventListener('drop', function(e){
    e.preventDefault();this.classList.remove('dragover');
    uploadFiles(e.dataTransfer.files);
  });
  document.getElementById('fileInput').addEventListener('change', function(){uploadFiles(this.files)});
}

function uploadFiles(files) {
  var prog = document.getElementById('uploadProgress');
  for (var i = 0; i < files.length; i++) {
    (function(file){
      var div = document.createElement('div');
      div.style.cssText = 'padding:8px 0;font-size:13px;color:#aaa';
      div.textContent = '上传中: ' + file.name + '...';
      prog.appendChild(div);

      var fd = new FormData();
      fd.append('file', file);
      fd.append('path', currentPath);
      var xhr = new XMLHttpRequest();
      xhr.open('POST', '/api/upload');
      xhr.setRequestHeader('X-Requested-With', 'XMLHttpRequest');
      xhr.onload = function() {
        if (xhr.status === 200) div.textContent = '✓ ' + file.name;
        else div.textContent = '✗ ' + file.name + ': 上传失败';
      };
      xhr.onerror = function(){ div.textContent = '✗ ' + file.name + ': 网络错误'; };
      xhr.send(fd);
    })(files[i]);
  }
}

function showLoginMsg(text, ok) {
  var el = document.getElementById('loginMsg');
  el.textContent = text;
  el.className = 'msg ' + (ok ? 'msg-ok' : 'msg-err');
}

function formatSize(b) {
  if (!b) return '0 B';
  var u = ['B','KB','MB','GB','TB'];
  var i = 0;
  while (b >= 1024 && i < 4) { b /= 1024; i++; }
  return b.toFixed(i?1:0) + ' ' + u[i];
}

function getFileIcon(name) {
  var ext = name.split('.').pop().toLowerCase();
  if (['mp4','mkv','avi','mov','wmv','flv'].indexOf(ext)>=0) return '&#127916;';
  if (['mp3','flac','wav','aac','ogg','m4a'].indexOf(ext)>=0) return '&#127925;';
  if (['jpg','jpeg','png','gif','bmp','webp','svg'].indexOf(ext)>=0) return '&#127748;';
  if (['pdf'].indexOf(ext)>=0) return '&#128196;';
  if (['zip','rar','7z','tar','gz'].indexOf(ext)>=0) return '&#128230;';
  if (['doc','docx'].indexOf(ext)>=0) return '&#128196;';
  if (['xls','xlsx'].indexOf(ext)>=0) return '&#128202;';
  if (['ppt','pptx'].indexOf(ext)>=0) return '&#128200;';
  if (['txt','md','json','xml','csv'].indexOf(ext)>=0) return '&#128196;';
  return '&#128196;';
}

function escHtml(s) {
  var d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

function escAttr(s) {
  return s.replace(/&/g,'&amp;').replace(/"/g,'&quot;').replace(/'/g,'&#39;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
}

// Init
checkStatus();
</script>
</body>
</html>`
