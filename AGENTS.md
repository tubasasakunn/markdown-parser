# Markfold の Go パーサー

このリポジトリは Markfold の Markdown パーサーです。iOS アプリと macOS アプリは、同じ [markdown-apps リポジトリ](https://github.com/tubasasakunn/markdown-apps) の別ターゲットです。この Go モジュールはそのリポジトリの `Dependencies/markdown-parser` にサブモジュールとして固定されています。

## 全体像と担当範囲

```text
Markdown ファイル
  → Go: 標準 Markdown と拡張記法を解析し、include / tasks を解決
  → Go Mobile: source-aware な AST を JSON で渡す
  → Swift: iOS / macOS の画面をネイティブ描画
  → Swift: Widget 用 snapshot を生成し、GitHub と同期
```

- Go は表示方法に依存しない AST、元ファイルの位置とハッシュ、タスク一覧、診断情報を作る。[`types.go`](types.go) の `Document.SchemaVersion` が Swift との JSON 契約。
- [`mobile/mobile.go`](mobile/mobile.go) の `ParseDirectory` と `SetTaskChecked` が Swift に公開する入口。後者は元ファイルのハッシュを検証して、変更後の Markdown 文字列を返す。ファイルへの書き込みは呼び出し側が行う。
- 画面描画、Widget、GitHub 認証・同期は Swift 側が担当する。`widget/WIDGET_TODO.md` と `widget/WIDGET_MEMO.md` はアプリの入口ファイル名で、Go パーサー固有の構文ではない。

## 関連リンク

| 対象 | 参照先 |
| --- | --- |
| iOS アプリの担当と実装 | [markdown-apps / AGENTS.md — iOS app](https://github.com/tubasasakunn/markdown-apps/blob/main/AGENTS.md#ios-app) |
| macOS アプリの担当と実装 | [markdown-apps / AGENTS.md — macOS app](https://github.com/tubasasakunn/markdown-apps/blob/main/AGENTS.md#macos-app) |
| パーサーの使い方・構文例 | [README.md](README.md) |
| 解析と拡張記法 | [parser.go](parser.go)、[directives.go](directives.go)、[goldmark.go](goldmark.go) |
| Go の検証 | [parser_test.go](parser_test.go)、[mobile/mobile_test.go](mobile/mobile_test.go) |
| アプリに配布する記法の説明 | [markdown-syntax.md](https://github.com/tubasasakunn/markdown-apps/blob/main/WorkspaceTemplate/docs/markdown-syntax.md) |

AST のフィールド、拡張記法、タスク編集の振る舞いを変えるときは、Swift 側の [`MarkdownDocument.swift`](https://github.com/tubasasakunn/markdown-apps/blob/main/Shared/MarkdownDocument.swift) と [`ParserBridge.swift`](https://github.com/tubasasakunn/markdown-apps/blob/main/App/ParserBridge.swift) も確認する。`go test ./...` を実行し、アプリ側のサブモジュール参照と XCFramework を更新して両ターゲットを検証する。
