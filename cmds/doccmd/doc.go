package doccmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/pubgo/redant"
	"github.com/pubgo/redant/cmds/vizcmd"
)

// New returns a "doc" command that serves an interactive documentation site
// generated from the command tree.
func New() *redant.Command {
	var (
		addr     string
		autoOpen bool
	)

	return &redant.Command{
		Use:   "doc",
		Short: "启动交互式命令文档站",
		Long:  "从命令树自动生成交互式文档站（类似 Swagger UI），包含命令结构、参数 Schema、Mermaid 流程图、调用示例，支持实时搜索与导航。",
		Options: redant.OptionSet{
			{
				Flag:        "addr",
				Description: "文档站监听地址",
				Value:       redant.StringOf(&addr),
				Default:     "127.0.0.1:18081",
			},
			{
				Flag:        "open",
				Description: "启动后自动打开浏览器",
				Value:       redant.BoolOf(&autoOpen),
				Default:     "true",
			},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			root := inv.Command
			for root.Parent() != nil {
				root = root.Parent()
			}

			listenAddr := strings.TrimSpace(addr)
			if listenAddr == "" {
				listenAddr = "127.0.0.1:18081"
			}

			ln, err := net.Listen("tcp", listenAddr)
			if err != nil {
				return err
			}
			defer func() { _ = ln.Close() }()

			url := "http://" + ln.Addr().String()
			_, _ = fmt.Fprintf(inv.Stdout, "doc site listening on %s\n", url)
			_, _ = fmt.Fprintf(inv.Stdout, "press Ctrl+C to stop\n")

			if autoOpen {
				if openErr := openBrowser(url); openErr != nil {
					_, _ = fmt.Fprintf(inv.Stderr, "open browser failed: %v\n", openErr)
				}
			}

			app := newDocApp(root)
			server := &http.Server{Handler: app.handler()}
			errCh := make(chan error, 1)
			go func() { errCh <- server.Serve(ln) }()

			select {
			case <-ctx.Done():
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				_ = server.Shutdown(shutdownCtx)
				return nil
			case err := <-errCh:
				if errors.Is(err, http.ErrServerClosed) {
					return nil
				}
				return err
			}
		},
	}
}

// --- doc app ---

type commandDoc struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Path        string            `json:"path"`
	Short       string            `json:"short,omitempty"`
	Long        string            `json:"long,omitempty"`
	Aliases     []string          `json:"aliases,omitempty"`
	Deprecated  string            `json:"deprecated,omitempty"`
	HasHandler  bool              `json:"hasHandler"`
	HandlerType string            `json:"handlerType,omitempty"` // "plain", "unary", "stream"
	Args        []argDoc          `json:"args,omitempty"`
	Options     []optionDoc       `json:"options,omitempty"`
	Children    []commandDoc      `json:"children,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type argDoc struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Default     string `json:"default,omitempty"`
}

type optionDoc struct {
	Flag        string   `json:"flag"`
	Shorthand   string   `json:"shorthand,omitempty"`
	Description string   `json:"description,omitempty"`
	Type        string   `json:"type,omitempty"`
	Default     string   `json:"default,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Envs        []string `json:"envs,omitempty"`
}

type docApp struct {
	root     *redant.Command
	treeJSON []byte // cached command tree JSON
	vizTree  string // cached Mermaid tree diagram
	vizFlow  string // cached Mermaid dispatch diagram
	vizMCP   string // cached Mermaid MCP sequence diagram
}

func newDocApp(root *redant.Command) *docApp {
	app := &docApp{root: root}

	// Pre-generate diagrams
	var buf bytes.Buffer
	_ = vizcmd.WriteTree(&buf, root, 0)
	app.vizTree = buf.String()

	buf.Reset()
	_ = vizcmd.WriteDispatch(&buf, root)
	app.vizFlow = buf.String()

	buf.Reset()
	_ = vizcmd.WriteMCPSequence(&buf, root, "")
	app.vizMCP = buf.String()

	// Pre-generate command tree
	tree := buildCommandDoc(root, "")
	app.treeJSON, _ = json.MarshalIndent(tree, "", "  ")

	return app
}

func (a *docApp) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleIndex)
	mux.HandleFunc("/api/tree", a.handleTree)
	mux.HandleFunc("/api/diagram/tree", a.handleDiagramTree)
	mux.HandleFunc("/api/diagram/dispatch", a.handleDiagramDispatch)
	mux.HandleFunc("/api/diagram/mcp", a.handleDiagramMCP)
	return mux
}

func (a *docApp) handleTree(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(a.treeJSON)
}

func (a *docApp) handleDiagramTree(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprint(w, a.vizTree)
}

func (a *docApp) handleDiagramDispatch(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprint(w, a.vizFlow)
}

func (a *docApp) handleDiagramMCP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprint(w, a.vizMCP)
}

func (a *docApp) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprint(w, docHTML)
}

func buildCommandDoc(cmd *redant.Command, prefix string) commandDoc {
	var path string
	if prefix == "" {
		path = cmd.Name()
	} else {
		path = prefix + ":" + cmd.Name()
	}

	doc := commandDoc{
		ID:         path,
		Name:       cmd.Name(),
		Path:       path,
		Short:      cmd.Short,
		Long:       cmd.Long,
		Aliases:    cmd.Aliases,
		Deprecated: cmd.Deprecated,
		Metadata:   cmd.Metadata,
	}

	switch {
	case cmd.ResponseStreamHandler != nil:
		doc.HasHandler = true
		doc.HandlerType = "stream"
	case cmd.ResponseHandler != nil:
		doc.HasHandler = true
		doc.HandlerType = "unary"
	case cmd.Handler != nil:
		doc.HasHandler = true
		doc.HandlerType = "plain"
	}

	for _, arg := range cmd.Args {
		ad := argDoc{
			Name:        arg.Name,
			Description: arg.Description,
			Required:    arg.Required,
			Default:     arg.Default,
		}
		if arg.Value != nil {
			ad.Type = arg.Value.Type()
		}
		doc.Args = append(doc.Args, ad)
	}

	for _, opt := range cmd.Options {
		if opt.Flag == "" || opt.Hidden {
			continue
		}
		od := optionDoc{
			Flag:        opt.Flag,
			Shorthand:   opt.Shorthand,
			Description: opt.Description,
			Default:     opt.Default,
			Required:    opt.Required,
			Envs:        opt.Envs,
		}
		if opt.Value != nil {
			od.Type = opt.Value.Type()
		}
		doc.Options = append(doc.Options, od)
	}

	for _, child := range cmd.Children {
		if child.Hidden {
			continue
		}
		doc.Children = append(doc.Children, buildCommandDoc(child, path))
	}

	return doc
}

func openBrowser(url string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return exec.Command(cmd, args...).Start()
}

// docHTML is the single-page documentation site.
const docHTML = `<!doctype html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1.0"/>
    <title>Command Documentation</title>
    <script src="https://cdn.tailwindcss.com"></script>
    <script defer src="https://cdn.jsdelivr.net/npm/alpinejs@3.x.x/dist/cdn.min.js"></script>
    <script type="module">
        import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.esm.min.mjs';
        mermaid.initialize({ startOnLoad: false, theme: 'dark' });
        window.mermaidAPI = mermaid;
    </script>
    <style>
        [x-cloak] { display: none !important; }
        .tree-item { transition: background-color 0.15s; }
        .tree-item:hover { background-color: rgba(59, 130, 246, 0.1); }
        .tree-item.active { background-color: rgba(59, 130, 246, 0.2); border-left: 3px solid #3b82f6; }
    </style>
</head>
<body class="h-screen overflow-hidden bg-slate-950 text-slate-100" x-data="docApp()" x-init="init()" x-cloak>
<div class="flex h-full">
    <!-- Sidebar: command tree -->
    <aside class="w-[320px] shrink-0 border-r border-slate-800 bg-slate-900/70 flex flex-col">
        <div class="p-3 border-b border-slate-800">
            <h1 class="text-sm font-semibold tracking-wide text-slate-200">Command Docs</h1>
            <input type="text" x-model.trim="query" placeholder="搜索命令..."
                class="mt-2 w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm outline-none focus:border-blue-500"/>
        </div>
        <div class="overflow-auto flex-1 py-1">
            <template x-for="node in filteredTree" :key="node.id">
                <div class="tree-item px-2 py-1.5 cursor-pointer"
                     :class="selected && selected.id === node.id ? 'active' : ''"
                     :style="'padding-left:' + (8 + node.depth * 16) + 'px'"
                     @click="selectNode(node)">
                    <div class="flex items-center gap-1.5">
                        <button type="button" class="w-4 text-[10px] text-slate-400 hover:text-slate-200"
                                @click.stop="toggleExpand(node)" x-show="node.children && node.children.length > 0"
                                x-text="node.expanded ? '▾' : '▸'"></button>
                        <span class="w-4 text-[10px] text-transparent" x-show="!node.children || node.children.length === 0">·</span>
                        <span class="text-sm" :class="node.hasHandler ? 'text-blue-400' : 'text-slate-400'" x-text="node.name"></span>
                        <span class="ml-auto text-[10px] px-1.5 py-0.5 rounded"
                              :class="{'bg-green-900/40 text-green-400': node.handlerType==='unary',
                                        'bg-purple-900/40 text-purple-400': node.handlerType==='stream',
                                        'bg-slate-800 text-slate-500': node.handlerType==='plain'}"
                              x-show="node.handlerType" x-text="node.handlerType"></span>
                    </div>
                    <div class="text-[11px] text-slate-500 ml-5 truncate" x-show="node.short" x-text="node.short"></div>
                </div>
            </template>
        </div>
    </aside>

    <!-- Main content -->
    <main class="flex-1 overflow-auto">
        <!-- Tab bar -->
        <div class="sticky top-0 z-10 border-b border-slate-800 bg-slate-900/90 backdrop-blur">
            <div class="flex">
                <template x-for="tab in tabs" :key="tab.id">
                    <button class="px-4 py-2.5 text-sm font-medium border-b-2 transition-colors"
                            :class="activeTab === tab.id ? 'border-blue-500 text-blue-400' : 'border-transparent text-slate-400 hover:text-slate-200'"
                            @click="activeTab = tab.id" x-text="tab.label"></button>
                </template>
            </div>
        </div>

        <!-- Tab: Command Detail -->
        <div x-show="activeTab === 'detail'" class="p-6 max-w-4xl">
            <template x-if="selected">
                <div>
                    <h2 class="text-2xl font-bold text-white" x-text="selected.path"></h2>
                    <p class="mt-1 text-slate-400" x-show="selected.short" x-text="selected.short"></p>
                    <p class="mt-3 text-sm text-slate-300 leading-relaxed whitespace-pre-wrap" x-show="selected.long" x-text="selected.long"></p>

                    <div class="mt-4 flex flex-wrap gap-2" x-show="selected.aliases && selected.aliases.length">
                        <template x-for="a in selected.aliases">
                            <span class="text-xs px-2 py-0.5 rounded bg-slate-800 text-slate-300" x-text="'alias: ' + a"></span>
                        </template>
                    </div>
                    <div class="mt-2 text-xs text-yellow-500" x-show="selected.deprecated" x-text="'⚠ Deprecated: ' + selected.deprecated"></div>

                    <!-- Args -->
                    <div class="mt-6" x-show="selected.args && selected.args.length">
                        <h3 class="text-sm font-semibold text-slate-200 mb-2">参数 (Args)</h3>
                        <div class="rounded-lg border border-slate-800 overflow-hidden">
                            <table class="w-full text-sm">
                                <thead class="bg-slate-900">
                                    <tr>
                                        <th class="text-left px-3 py-2 text-slate-400 font-medium">名称</th>
                                        <th class="text-left px-3 py-2 text-slate-400 font-medium">类型</th>
                                        <th class="text-left px-3 py-2 text-slate-400 font-medium">必填</th>
                                        <th class="text-left px-3 py-2 text-slate-400 font-medium">默认值</th>
                                        <th class="text-left px-3 py-2 text-slate-400 font-medium">说明</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    <template x-for="arg in selected.args">
                                        <tr class="border-t border-slate-800">
                                            <td class="px-3 py-2 text-blue-400 font-mono text-xs" x-text="arg.name"></td>
                                            <td class="px-3 py-2 text-slate-400 text-xs" x-text="arg.type || '-'"></td>
                                            <td class="px-3 py-2" x-text="arg.required ? '✔' : ''"></td>
                                            <td class="px-3 py-2 text-slate-500 text-xs font-mono" x-text="arg.default || '-'"></td>
                                            <td class="px-3 py-2 text-slate-300 text-xs" x-text="arg.description || ''"></td>
                                        </tr>
                                    </template>
                                </tbody>
                            </table>
                        </div>
                    </div>

                    <!-- Options -->
                    <div class="mt-6" x-show="selected.options && selected.options.length">
                        <h3 class="text-sm font-semibold text-slate-200 mb-2">选项 (Options)</h3>
                        <div class="rounded-lg border border-slate-800 overflow-hidden">
                            <table class="w-full text-sm">
                                <thead class="bg-slate-900">
                                    <tr>
                                        <th class="text-left px-3 py-2 text-slate-400 font-medium">Flag</th>
                                        <th class="text-left px-3 py-2 text-slate-400 font-medium">类型</th>
                                        <th class="text-left px-3 py-2 text-slate-400 font-medium">默认值</th>
                                        <th class="text-left px-3 py-2 text-slate-400 font-medium">Env</th>
                                        <th class="text-left px-3 py-2 text-slate-400 font-medium">说明</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    <template x-for="opt in selected.options">
                                        <tr class="border-t border-slate-800">
                                            <td class="px-3 py-2 font-mono text-xs">
                                                <span class="text-green-400" x-text="'--' + opt.flag"></span>
                                                <span class="text-slate-500 ml-1" x-show="opt.shorthand" x-text="'-' + opt.shorthand"></span>
                                                <span class="ml-1 text-red-400 text-[10px]" x-show="opt.required">required</span>
                                            </td>
                                            <td class="px-3 py-2 text-slate-400 text-xs" x-text="opt.type || '-'"></td>
                                            <td class="px-3 py-2 text-slate-500 text-xs font-mono" x-text="opt.default || '-'"></td>
                                            <td class="px-3 py-2 text-slate-500 text-xs font-mono" x-text="opt.envs ? opt.envs.join(', ') : '-'"></td>
                                            <td class="px-3 py-2 text-slate-300 text-xs" x-text="opt.description || ''"></td>
                                        </tr>
                                    </template>
                                </tbody>
                            </table>
                        </div>
                    </div>

                    <!-- Metadata -->
                    <div class="mt-6" x-show="selected.metadata && Object.keys(selected.metadata).length">
                        <h3 class="text-sm font-semibold text-slate-200 mb-2">Metadata</h3>
                        <div class="rounded-lg border border-slate-800 p-3 bg-slate-900/50">
                            <template x-for="[k,v] in Object.entries(selected.metadata || {})">
                                <div class="text-xs py-0.5">
                                    <span class="text-slate-400" x-text="k + ': '"></span>
                                    <span class="text-slate-200" x-text="v"></span>
                                </div>
                            </template>
                        </div>
                    </div>
                </div>
            </template>
            <div x-show="!selected" class="text-slate-500 text-sm">← 选择一个命令查看详情</div>
        </div>

        <!-- Tab: Command Tree Diagram -->
        <div x-show="activeTab === 'tree-diagram'" class="p-6">
            <h2 class="text-lg font-semibold text-slate-200 mb-4">命令树结构图</h2>
            <div id="mermaid-tree" class="bg-slate-900/50 rounded-lg p-4 overflow-auto"></div>
        </div>

        <!-- Tab: Dispatch Flow -->
        <div x-show="activeTab === 'dispatch'" class="p-6">
            <h2 class="text-lg font-semibold text-slate-200 mb-4">命令分发流程</h2>
            <div id="mermaid-dispatch" class="bg-slate-900/50 rounded-lg p-4 overflow-auto"></div>
        </div>

        <!-- Tab: MCP Sequence -->
        <div x-show="activeTab === 'mcp'" class="p-6">
            <h2 class="text-lg font-semibold text-slate-200 mb-4">MCP 调用时序</h2>
            <div id="mermaid-mcp" class="bg-slate-900/50 rounded-lg p-4 overflow-auto"></div>
        </div>
    </main>
</div>

<script>
function docApp() {
    return {
        tree: null,
        flatNodes: [],
        selected: null,
        query: '',
        activeTab: 'detail',
        tabs: [
            { id: 'detail', label: '命令详情' },
            { id: 'tree-diagram', label: '命令树图' },
            { id: 'dispatch', label: '分发流程' },
            { id: 'mcp', label: 'MCP 时序' },
        ],
        diagramsRendered: {},

        async init() {
            const res = await fetch('/api/tree');
            this.tree = await res.json();
            this.flatNodes = this.flatten(this.tree, 0, true);
            if (this.flatNodes.length > 0) {
                this.selectNode(this.flatNodes[0]);
            }

            this.$watch('activeTab', (tab) => this.renderDiagram(tab));
        },

        flatten(node, depth, expanded) {
            const items = [];
            const n = { ...node, depth, expanded };
            items.push(n);
            if (expanded && node.children) {
                for (const child of node.children) {
                    items.push(...this.flatten(child, depth + 1, false));
                }
            }
            return items;
        },

        get filteredTree() {
            if (!this.query) return this.flatNodes;
            const q = this.query.toLowerCase();
            return this.flatNodes.filter(n =>
                n.name.toLowerCase().includes(q) ||
                (n.short || '').toLowerCase().includes(q) ||
                (n.path || '').toLowerCase().includes(q) ||
                (n.aliases || []).some(a => a.toLowerCase().includes(q))
            );
        },

        selectNode(node) {
            this.selected = node;
        },

        toggleExpand(node) {
            node.expanded = !node.expanded;
            this.rebuildFlat();
        },

        rebuildFlat() {
            if (!this.tree) return;
            const rebuild = (node, depth) => {
                const items = [];
                const existing = this.flatNodes.find(n => n.id === node.id);
                const expanded = existing ? existing.expanded : false;
                const n = { ...node, depth, expanded };
                items.push(n);
                if (expanded && node.children) {
                    for (const child of node.children) {
                        items.push(...rebuild(child, depth + 1));
                    }
                }
                return items;
            };
            this.flatNodes = rebuild(this.tree, 0);
        },

        async renderDiagram(tab) {
            const mapping = {
                'tree-diagram': { el: 'mermaid-tree', api: '/api/diagram/tree' },
                'dispatch': { el: 'mermaid-dispatch', api: '/api/diagram/dispatch' },
                'mcp': { el: 'mermaid-mcp', api: '/api/diagram/mcp' },
            };
            const cfg = mapping[tab];
            if (!cfg || this.diagramsRendered[tab]) return;

            const res = await fetch(cfg.api);
            const src = await res.text();
            const el = document.getElementById(cfg.el);
            if (!el) return;

            try {
                const { svg } = await window.mermaidAPI.render(cfg.el + '-svg', src);
                el.innerHTML = svg;
                this.diagramsRendered[tab] = true;
            } catch (e) {
                el.innerHTML = '<pre class="text-red-400 text-xs">' + e.message + '</pre><pre class="text-slate-500 text-xs mt-2">' + src + '</pre>';
            }
        },
    };
}
</script>
</body>
</html>`
