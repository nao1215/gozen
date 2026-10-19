# ADR 0019: エンジンの文言を英語と日本語で持ち、game.yaml の language で選ぶ

状態: 採用。日付: 2026-10-19。`text` パッケージ（第 1 層）、`data.Game`、`gozen` の文書を作るサブコマンドに適用します。

## 利用者と問題

利用者は Gozen で英語または日本語のゲームを作る作者です。作者の決定（2026-10-19）は「Gozen は日本語と英語で対応できるようにして」です。

これまでエンジンが画面に出す文言（メニュー、戦闘のメッセージ、設定、セーブ、ギャラリー、開発用の表示）は日本語の原文だけで、英語のゲームは game.yaml の `text` で全部を置き換える必要がありました。漏れた文言は日本語のまま出て、英語の字体では豆腐になります。Wayfinder は 80 行あまりの置き換えを持ち、それでも設定や一部の表示が日本語でした。`gozen book` などの CLI が作るゲームの文書も日本語の見出しでした。

## 決定

- 文言のキーは今のまま、エンジンの日本語の原文（書式の `%d`・`%s` を含む）にします。既存の game.yaml の `text`（rpg の上書きを含む）がそのまま効きます。
- 組み込みの英語の一覧を `internal/text/catalog` に置きます。日本語の一覧は各キーそのものです。テスト（`internal/text/catalog`）がモジュールのコードを構文木で読み、`text.T` に渡す全キーと英語の一覧が一致すること（足りないキー・使われないキーがないこと）、英語が同じ引数を同じ動詞で読み、`{name}` を揃え、日本語の字を含まないことを確かめます。キーが文字列リテラルでない呼び出しは、テストの表に置き場所と取り得るキーを書かないと失敗します。
- game.yaml に `language` を足します。値は `en`（英語）と `ja`（日本語）で、省略は `en` です。`text` は選んだ言語の文言の上に重ねます。それ以外の値は `gozen check` の誤りです。
- `text.Use(language, words)` が `Use(words)` に替わり、`text.English`・`text.Japanese` を公開します。最初の `Use` の前は `T` がキーをそのまま返します（今までと同じ）。英語の一覧を読む `Words` は公開しません。使うのは読み込み（既定の装備枠の名前）と `gozen check` の字体の検査だけだからです。
- runner は game.yaml を読む前に `language` と `text` だけを先に読み、読み込みの診断もゲームの言語で出します（`gamedata.UseWords`）。`gozen` の check・layout・book・design・sim・curve・floors・trial・shots も同じです。ゲームのディレクトリを取らない `gozen memos`・`gozen trace` は、メモとトレースの開始行に記録した `language` に従います。記録の無い古いメモ・トレースは日本語で書かれた物として扱います。
- `gozen` 自身の診断（release・trial・shots の誤りと要約）は英語にします。ゲームの文書（book の見出し、design の通信簿、memos・shots の HTML、trace の読み取り）は `text.T` を通し、ゲームの言語に従います。
- `gozen check` は、`language` を省いたのに題名・副題・`text` が日本語のときに `language: ja` を勧める警告を出します。`fonts.regular` が選んだ言語の文言と `text` の文字（字と数字だけ。記号は字体の選択とみなす）の字形を持たないときも警告します。

## 代案

1. キーを `battle.cannot_escape` のような ID に替える案。翻訳の管理はしやすくなりますが、既存の game.yaml の `text`（作品の上書き）が全部効かなくなります。作者の rpg と Wayfinder を書き直す必要があり、採りません。
2. 省略時の既定を日本語にする案。既存の日本語のゲームは何もしなくて済みますが、公開の枠組みとして英語の作者が最初に日本語を見ることになります。既定は英語とし、日本語のゲームには `language: ja` の 1 行を求め、`gozen check` の警告で気付けるようにします。
3. 英語の一覧を YAML で埋め込む案。第 1 層の `text` が YAML の解析に依存し、キーと書式の検査も別に要ります。Go の map にしてテストで検査します。

## 影響

- 公開 API: `text.Use` の引数が変わり、`text.English`・`text.Japanese`・`data.Game.Language` が増えます（`api/public.txt`）。スキーマに `Game.language` が増えます（`docs/schema.md`、`data/schema/types.schema.json`）。
- 既存の日本語のゲームは game.yaml に `language: ja` を足すまで、エンジンの文言が英語になります。CHANGELOG と `gozen check` の警告で知らせます。
- メモ（`memo.Memo.Language`）とトレースの開始行に `language` が増えます。省略可能な項目で、古い記録も読めます。
- ギャラリーの頁選択の「はじめから」は、タイトルの「はじめから」と英語が別になるため、キーを「最初から」に替えました。
- 文言を足すときは英語の一覧にも足します。足し忘れはテストで失敗します。
