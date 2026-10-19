# 動く立ち絵（puppet）

一枚絵のキャラクターを、まばたき・口の動き・呼吸・頭の揺れと首かしげ・髪の揺れ・一回だけの反応・表情の切り替えで動かす。判断と代案は [ADR 0020](../decisions/0020-moving-portraits.md)。

## 使い方

```go
rig, err := puppet.Load(assets, "streamer/puppet.yaml") // YAML と PNG を読み、検証する
if err != nil { return err }
p, err := puppet.New(rig, puppet.Config{Rand: rand.New(rand.NewPCG(seed, 0))})
if err != nil { return err }

// Update の中
p.Update()
if lineShown { p.Speak() } else { p.Stop() }
if gift { p.React("hop"); p.SetFace("surprised") }

// Draw の中（背景の後、UI の前）
w, h := rig.Size()
op := &ebiten.DrawImageOptions{}
s := 520 / float64(h)
op.GeoM.Scale(s, s)
op.GeoM.Translate(x-float64(w)*s/2, screenH-float64(h)*s)
p.Draw(screen, op)
```

Puppet はゲームのスレッドだけで使う。`Update` は 1 回で `1/TPS` 秒進む（`Config.TPS` の既定は Ebitengine の既定の 60）。同じ乱数の種と同じ呼び出しの列なら同じ動きになる。画面から外すときは `Close`。

頭と髪は基準の絵の範囲から頭の半径の数割まではみ出すことがあり、跳ねる・揺れるは絵全体を動かす。配置では上と左右に少し余白を取る。

## RPG の会話の立ち絵

仲間（actors）に `puppet` を書くと、会話の窓の立ち絵がその puppet になる。

```yaml
actors:
  walker: {name: Walker, class: walker, faces: walker, puppet: walker/puppet.yaml}
```

- パスは `assets/` からの相対。puppet の表情の名前は、台詞の表情（`assets/<faces>/<表情>.png` の `<表情>`）と同じにする。puppet に無い表情の台詞は、これまでどおり表情の絵を出す。
- 文字が出ている間だけ口を動かし、同じ人の続く台詞では表情をふわっと替える。大きさの扱いは背の高い立ち絵と同じ（画面の高さに合わせる）。
- 乱数は仲間の ID から決まり、ゲームの乱数を使わない（記録の再現に影響しない）。F5 の読み直しで作り直す。
- `data.Load`（`gozen check --no-layout`）は YAML・絵のファイルの有無・仲間の `face` が puppet にあることを、`gozen check` はさらに絵の大きさと位置を確かめる。
- メニュー・戦闘・カットインの立ち絵は動かない（表情の絵のまま）。

## 用意する絵

| 絵 | 中身 |
| --- | --- |
| 基準の絵 | 透過 PNG。表情の一つ（多くは普段の顔）を兼ねる |
| 閉じた目 | 目を閉じた絵。基準と同じ大きさの全体の絵でも、目の所だけの小さな絵（`at` で置く場所）でもよい |
| 口の形 | 閉じ・半開きなど。基準の絵の口も 1 つの形として使える（`face`） |
| 別の表情 | 全体の絵か、違う所だけの小さな絵 |

全体の絵は、読み込むときに基準（表情の目や口なら、その表情の絵）と比べて違う所だけを切り出し、縁を 16 画素ほどでぼかす。画像生成で全体を描き直した絵は全体が少しずつ違うので、`area`（`[x, y, w, h]`）で探す範囲を目や口の周りに絞る。透明の無い絵（背景つきで描いた絵）は、下の絵の透明をそのまま使う。

`gozen puppet <puppet.yaml>` で各差分の位置を確かめられる。`--out DIR` は差分を切り出した小さな絵と `at` 付きの YAML を書くので、配布物が小さくなる（書いた YAML に注釈は残らない）。

## YAML

座標はすべて基準の絵の画素（左上が原点）。絵のパスは YAML のあるディレクトリからの相対パス。未知のキーは誤り。

```yaml
base: streamer.png
face: smile              # 最初の表情。表情が 2 つ以上なら必須
fade: 0.25               # 表情の切り替えの秒
faces:
  smile: {}              # 基準の絵のまま
  surprised:
    image: surprised.png # 全体の絵（違う所を切り出す）
    area: [420, 380, 520, 420]
    eyes: {off: true}    # この表情はまばたきしない（目の形が違う）
eyes:
  closed: {image: eyes_closed.png, area: [420, 490, 560, 170]}
  every: [2, 5.5]        # まばたきの間隔の秒（最短・最長）
  double: 0.25           # 続けて 2 回まばたきする確率
mouth:
  frames:                # 話している間、不規則に替える（2 つ以上）
    - {image: mouth_closed.png, at: [564, 714]}  # 小さな絵は at で置く
    - mouth_half.png                            # 全体の絵はパスだけでもよい
    - face                                      # 表情の絵の口そのまま
  speed: 11              # 1 秒に替える回数
head:
  center: [690, 520]     # 頭の中心。円が髪を含む頭を覆うように
  radius: 400
  neck: [690, 1090]      # 首の付け根。頭はここを軸に回る。center より下
hair:                    # 頭に遅れて揺れる髪。下の方ほど大きく動く
  - rect: [200, 330, 290, 700]          # [x, y, w, h]
  - poly: [[860, 330], [1190, 330], [1190, 1030], [860, 1030]]
motion:                  # 省略した組は既定値。組の中の強さ 0 はその動きを止める
  breath: {depth: 0.006, period: 3.8}   # 縦の伸縮（高さの割合）と周期の秒
  sway: {angle: 2.5, shift: 10, period: 6.5}  # 頭の揺れの角度（度）・ずれ（画素）
  tilt: {angle: 7, every: [5, 11]}      # たまの首かしげ
  hair: {gain: 1, stiffness: 60, damping: 7}  # 髪のばね
  talk: {nod: 8}                        # 話している間の頭の上下（画素）
reactions:               # 名前を付けた強さ違い。組み込みの名前はそのまま使える
  jump: {kind: hop, amount: 1.6}
  nod: {time: 0.7}
```

| 項目 | 既定 | 検証 |
| --- | --- | --- |
| `faces` | 省略時は `default` 1 つ（基準の絵） | `at`・`area` は `image` と一緒に。`face` は faces の名前 |
| `eyes`・`mouth` | なし（動かない） | 表情ごとに上書きでき、`off: true` で止める。`frames` は 2 つ以上 |
| `eyes.every`・`double` | `[2, 6]`・0.2 | 正の昇順・0〜1 |
| `mouth.speed` | 10 | 0 以上 |
| `head` | なし（頭・髪・首かしげ・話す動きなし） | `radius` は正、`neck` は `center` より下 |
| `hair` | なし | `head` が要る。矩形は正の大きさ、多角形は 3 点以上 |
| `motion.sway.shift`・`talk.nod` | 頭の半径の 2.5%・2% | 0 以上。角度は 45 度まで、`breath.depth` は 0.2 まで |
| `reactions` | 組み込みの hop・shrink・shake・nod・tilt | `kind` は組み込みのどれか。`amount`（倍率、既定 1）と `time`（秒）は 0 以上 |

絵の誤り（無い、PNG でない、8192 画素より大きい、基準の外にはみ出す、全体の絵の大きさが基準と違う、違う所が無い）は `Load` が場所付きで返す。
