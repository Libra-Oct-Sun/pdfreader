# SimpleReader

[![CI](https://github.com/Libra-Oct-Sun/pdfreader/actions/workflows/ci.yml/badge.svg)](https://github.com/Libra-Oct-Sun/pdfreader/actions/workflows/ci.yml)

一个面向 Windows 的轻量 PDF 阅读器：**打开即续读，笔落即批注，退出即保存**。

鼠标就是笔。没有压感、掌拒和复杂批注管理，重点是把阅读位置、鼠标批注和文档导航做扎实。

## 已实现

- 打开 PDF、最近文件、拖拽打开、命令行打开
- 连续滚动、适宽显示、缩放、页码跳转
- 鼠标钢笔、荧光笔、整笔橡皮、颜色、线宽、撤销/重做
- 阅读位置自动保存与恢复（页码 + 页内偏移 + 缩放）
- 笔迹按页面归一化坐标保存，缩放后仍然对齐
- PDF 目录、手动书签和批注页面侧栏
- 导出带矢量批注的 PDF，保留原文字层
- 本地 JSON 存储，不上传任何文件
- 加密 PDF 密码输入
- 用户级安装包、PDF 文件关联和单实例打开
- 首次打开只读取第一页尺寸，其余页面按需校正

## 技术栈

- Go + Wails v2：窗口、文件、阅读状态、笔迹存储、本地 PDF 读取
- WebView2：Windows 自带的 Chromium 内核，不打包完整浏览器
- Svelte 5 + TypeScript + Vite：轻量界面
- pdf.js：PDF 渲染和文本字体处理
- Canvas：鼠标墨迹层
- pdf-lib：把笔迹以矢量路径写入导出的 PDF

PDF 文件通过 Go 启动的 `127.0.0.1` 临时只读服务传给 pdf.js，避免把整份 PDF 转成 base64 占用额外内存。

## 开发

环境要求：

- Go 1.22+
- Node.js 20+
- WebView2 Runtime（Windows 11 自带，Windows 10 一般也已安装）
- NSIS（只有构建安装包时需要）

```powershell
cd frontend
npm install
cd ..
wails dev
```

生产构建：

```powershell
wails build
```

产物在 `build/bin/reader.exe`。

构建带文件关联的安装包：

```powershell
wails build -clean -nsis -installscope user
```

产物包括 `build/bin/reader.exe` 和 `build/bin/SimpleReader-amd64-installer.exe`。安装包会注册 PDF 打开方式，卸载时恢复安装前的关联。

图标源文件在 `build/icon.svg`，重新生成 `appicon.png` 和 Windows ICO：

```powershell
python scripts/generate_icon.py
```

本地一次跑完所有检查：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/check.ps1
```

## 测试与发布

- 推送到 `main` 或提交 Pull Request：自动运行 Go 测试、Svelte 检查、前端生产构建和 Windows 应用构建
- 推送 `v*` 标签：自动构建便携版、NSIS 安装包、zip、生成 SHA-256，并创建 GitHub Release
- Release 产物包括便携版 exe、安装包、zip 和校验文件

创建存档版本：

```powershell
git tag -a v0.2.0 -m "SimpleReader v0.2.0"
git push origin v0.2.0
```

## 使用

- `P`：进入/退出批注模式
- `Esc`：退出批注模式
- 鼠标左键拖动：在批注模式下画线
- 鼠标滚轮：翻页
- `Ctrl + 滚轮`：缩放
- `Ctrl + Z` / `Ctrl + Y`：撤销 / 重做
- `Ctrl + D`：收藏/取消当前页书签
- `Ctrl + B`：打开/关闭目录、书签和批注侧栏
- `Ctrl + E`：导出带批注 PDF
- 右下角工具栏：钢笔、荧光笔、橡皮、颜色、线宽、撤销、完成

## 数据位置

```text
%APPDATA%/SimpleReader/
  library.json          # 最近文档和阅读位置
  notes/<docId>.json    # 每份文档的鼠标笔迹
  bookmarks/<docId>.json # 每份文档的手动书签
```

文档 ID 是 PDF 内容的 SHA-256，文件改名或移动后仍能识别阅读位置。

## 当前边界

当前版本不做账号、云同步、OCR、文本选择/搜索、多标签和手写转文字。

后续比较自然的方向：

1. 文本搜索、选择与复制
2. 荧光笔吸附到文本行
3. 套索选择、移动和删除批注
4. 扫描件 OCR
5. 用 Go 侧 PDFium 渲染替换 pdf.js，进一步压缩内存
