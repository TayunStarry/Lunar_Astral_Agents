package storyboard

// slide.go 演绎最终产物：用模板拼接一份自包含的幻灯片 HTML。
//
// 组成方式：
//   - 每一镜对应一张 slide，图片取本镜画面，配音取本镜原文合成的 WAV；
//   - 每张 slide 的展示时长直接等于该镜配音的播放时长（无配音时按字数估算）；
//   - slide 之间的切换由页面内的脚本随机选择转场效果，不做硬切；
//   - 页面只引用相对路径的图片与音频，不依赖任何外部 CDN。
//
// 图片在 multimedia/generated/<标题>/，配音在 audios/generated/<标题>/，
// HTML 落在 documents/lens/<标题>.html，
// 因此相对路径固定为 ../../multimedia/generated/<标题>/... 与 ../../audios/generated/<标题>/...

import (
	"fmt"
	"html/template"
	"os"
	"path"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/agents/file/FileEditor"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// slideItem 一张幻灯片：画面、配音与展示时长。
type slideItem struct {
	// Title 镜次标题（用于无障碍描述）
	Title string
	// Image 相对 HTML 的图片路径
	Image string
	// Audio 相对 HTML 的配音路径（无配音时为空）
	Audio string
	// DurationMs 展示时长（毫秒），等于配音播放时长
	DurationMs int
}

// slidePage 幻灯片页面的渲染数据。
type slidePage struct {
	Title  string
	Count  int
	Slides []slideItem
}

// writeStoryboardSlide 由分镜与配音结果拼接幻灯片 HTML，并把镜头映射为相对路径。
func writeStoryboardSlide(plan storyboardPlan, shots []ShotView, folder string) (docResult, []ShotView, error) {
	if folder == "" {
		folder = prepareOutputFolder(plan.Title)
	}
	docDirRel := docRootRel
	for index := range shots {
		if shots[index].Status != shotPainted || shots[index].Handle == "" {
			continue
		}
		link, err := shotImageLink(docDirRel, shots[index].Handle)
		if err != nil {
			shots[index].Status = shotFailed
			shots[index].Error = "图片路径解析失败：" + err.Error()
			shots[index].File = ""
			continue
		}
		shots[index].File = link
	}

	items := make([]slideItem, 0, len(shots))
	for _, shot := range shots {
		if shot.Status != shotPainted || shot.File == "" {
			continue
		}
		audio := ""
		if shot.VoiceStatus == voiceReady {
			if link, err := voiceLink(docDirRel, shot); err == nil {
				audio = link
			}
		}
		duration := shot.VoiceDurationMs
		if duration <= 0 {
			// 无配音时按原文（缺失则按叙事）的字数估算，保证幻灯片仍按节奏推进。
			estimateText := strings.TrimSpace(shot.Original)
			if estimateText == "" {
				estimateText = shot.Narration
			}
			duration = estimateSpeechMs(estimateText)
		}
		items = append(items, slideItem{
			Title:      shotSlideTitle(shot),
			Image:      shot.File,
			Audio:      audio,
			DurationMs: duration,
		})
	}
	if len(items) == 0 {
		return docResult{}, shots, fmt.Errorf("没有任何分镜产出可用画面，无法拼接幻灯片")
	}

	content, err := renderSlidePage(slidePage{
		Title:  plan.Title,
		Count:  len(items),
		Slides: items,
	})
	if err != nil {
		return docResult{}, shots, fmt.Errorf("幻灯片模板渲染失败：%w", err)
	}
	docRel := path.Join(docDisplayRoot, folder+slideFileExt)
	result := FileEditor.CreateFile(docRel, content)
	if !result.OK {
		return docResult{}, shots, fmt.Errorf("幻灯片写入失败：%s", strings.TrimSpace(result.Summary))
	}
	handle := ""
	if len(result.Handles) > 0 {
		handle = result.Handles[0]
	}
	return docResult{
		Path:   path.Join(docRootRel, folder+slideFileExt),
		Handle: handle,
	}, shots, nil
}

// voiceLink 把配音实体解析为相对幻灯片目录的链接。
func voiceLink(docDirRel string, shot ShotView) (string, error) {
	rel := strings.TrimSpace(shot.VoiceFile)
	if shot.VoiceHandle != "" {
		if entry, err := resource.Resolve(shot.VoiceHandle); err == nil {
			rel = strings.TrimSpace(entry.BlobPath)
		}
	}
	if rel == "" {
		return "", fmt.Errorf("配音没有本地文件")
	}
	srcRel := filepath.ToSlash(filepath.Clean(rel))
	if _, err := os.Stat(filepath.Join(config.LocalDir, filepath.FromSlash(srcRel))); err != nil {
		return "", fmt.Errorf("配音不存在：%w", err)
	}
	link, err := filepath.Rel(filepath.FromSlash(docDirRel), filepath.FromSlash(srcRel))
	if err != nil {
		return "", fmt.Errorf("计算配音相对路径失败：%w", err)
	}
	return filepath.ToSlash(link), nil
}

// shotSlideTitle 生成单镜的可读标题。
func shotSlideTitle(shot ShotView) string {
	title := strings.TrimSpace(shot.Scene)
	if title == "" {
		title = fmt.Sprintf("第 %d 镜", shot.Index)
	}
	if act := strings.TrimSpace(shot.Act); act != "" {
		title = act + " · " + title
	}
	return title
}

// renderSlidePage 用固定模板渲染幻灯片页面。
func renderSlidePage(page slidePage) (string, error) {
	tpl, err := template.New("slide").Parse(slideHTMLTemplate)
	if err != nil {
		return "", err
	}
	var builder strings.Builder
	if err := tpl.Execute(&builder, page); err != nil {
		return "", err
	}
	return builder.String(), nil
}

// slideHTMLTemplate 幻灯片模板：页面骨架 + 逐镜 slide 片段（由 range 拼接）+ 随机转场脚本。
const slideHTMLTemplate = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
<title>{{.Title}} · 演绎幻灯片</title>
<style>
  :root{
    --ink:#f6f8ff;
    --dim:rgba(246,248,255,.6);
    --accent:#8fb8ff;
  }
  *{margin:0;padding:0;box-sizing:border-box}
  html,body{height:100%;background:#05060a;overflow:hidden}
  body{
    font-family:"PingFang SC","Microsoft YaHei","Segoe UI",system-ui,-apple-system,sans-serif;
    color:var(--ink);
  }
  #stage{
    position:fixed;inset:0;overflow:hidden;
    background:radial-gradient(120% 90% at 50% 35%,#0d1324 0%,#05060a 68%);
  }
  .slide{
    position:absolute;inset:0;display:flex;align-items:center;justify-content:center;
    opacity:0;pointer-events:none;will-change:transform,opacity,filter;
  }
  .slide.is-active{opacity:1;pointer-events:auto}
  .slide img{
    width:100%;height:100%;object-fit:contain;display:block;
    transform:scale(1);animation:drift 20s linear both;
    user-select:none;-webkit-user-drag:none;
  }
  @keyframes drift{from{transform:scale(1.01)}to{transform:scale(1.08)}}
  #hud{
    position:fixed;left:0;right:0;bottom:0;padding:26px 36px 22px;
    display:flex;align-items:center;gap:18px;
    background:linear-gradient(to top,rgba(3,5,10,.82),rgba(3,5,10,0));
    opacity:0;transition:opacity .6s ease;pointer-events:none;
  }
  body.playing #hud{opacity:1}
  #hud .title{
    font-size:12px;letter-spacing:.32em;color:var(--dim);
    white-space:nowrap;max-width:32vw;overflow:hidden;text-overflow:ellipsis;
  }
  #hud .bar{flex:1;height:2px;background:rgba(255,255,255,.12);border-radius:2px;overflow:hidden}
  #hud .bar i{display:block;height:100%;width:0;background:linear-gradient(90deg,var(--accent),#c9a6ff);border-radius:2px}
  #hud .count{font-size:12px;color:var(--dim);font-variant-numeric:tabular-nums}
  #cover{
    position:fixed;inset:0;z-index:9;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:20px;
    background:radial-gradient(90% 70% at 50% 40%,rgba(20,28,52,.92),rgba(4,5,9,.97));
    transition:opacity .8s ease;
  }
  #cover.is-hidden{opacity:0;pointer-events:none}
  #cover .kicker{font-size:11px;letter-spacing:.52em;color:var(--dim)}
  #cover h1{
    font-size:clamp(26px,4.6vw,54px);font-weight:600;letter-spacing:.08em;
    text-align:center;max-width:86vw;
  }
  #cover p{font-size:13px;color:var(--dim);letter-spacing:.16em}
  #cover button{
    margin-top:6px;padding:13px 46px;border-radius:999px;cursor:pointer;
    border:1px solid rgba(255,255,255,.24);background:rgba(255,255,255,.06);
    color:var(--ink);font-size:14px;letter-spacing:.34em;
    backdrop-filter:blur(14px);
    transition:transform .25s ease,background .25s ease;
  }
  #cover button:hover{transform:translateY(-2px);background:rgba(255,255,255,.12)}
  #cover button:active{transform:translateY(0)}
</style>
</head>
<body>
<div id="stage">
{{range .Slides}}<section class="slide" data-duration="{{.DurationMs}}"{{if .Audio}} data-audio="{{.Audio}}"{{end}}><img src="{{.Image}}" alt="{{.Title}}" loading="eager" decoding="async"></section>
{{end}}</div>
<div id="hud">
  <span class="title">{{.Title}}</span>
  <span class="bar"><i id="bar"></i></span>
  <span class="count" id="count">1 / {{.Count}}</span>
</div>
<div id="cover">
  <span class="kicker">LUNAR STORYBOARD</span>
  <h1>{{.Title}}</h1>
  <p>{{.Count}} 个分镜 · 按旁白时长自动推进</p>
  <button id="start" type="button">开始播放</button>
</div>
<script>
(function(){
  var slides = Array.prototype.slice.call(document.querySelectorAll('.slide'));
  var bar = document.getElementById('bar');
  var countLabel = document.getElementById('count');
  var cover = document.getElementById('cover');
  var startButton = document.getElementById('start');
  if (!slides.length) return;

  // 随机转场：进场与退场成对定义，保证画面之间是叠加过渡而不是硬切。
  var effects = [
    {in:[{opacity:0,transform:'scale(1.18)',filter:'blur(18px)'},{opacity:1,transform:'scale(1)',filter:'blur(0px)'}],
     out:[{opacity:1,transform:'scale(1)'},{opacity:0,transform:'scale(.92)',filter:'blur(12px)'}]},
    {in:[{opacity:0,transform:'translate3d(16%,0,0) scale(1.08)'},{opacity:1,transform:'translate3d(0,0,0) scale(1)'}],
     out:[{opacity:1,transform:'translate3d(0,0,0)'},{opacity:0,transform:'translate3d(-12%,0,0)'}]},
    {in:[{opacity:0,transform:'translate3d(-16%,0,0) scale(1.08)'},{opacity:1,transform:'translate3d(0,0,0) scale(1)'}],
     out:[{opacity:1,transform:'translate3d(0,0,0)'},{opacity:0,transform:'translate3d(12%,0,0)'}]},
    {in:[{opacity:0,transform:'translate3d(0,14%,0) scale(1.1)',filter:'blur(10px)'},{opacity:1,transform:'translate3d(0,0,0) scale(1)',filter:'blur(0px)'}],
     out:[{opacity:1,transform:'translate3d(0,0,0)'},{opacity:0,transform:'translate3d(0,-10%,0)'}]},
    {in:[{opacity:0,transform:'rotate(2.4deg) scale(1.16)'},{opacity:1,transform:'rotate(0deg) scale(1)'}],
     out:[{opacity:1,transform:'rotate(0deg) scale(1)'},{opacity:0,transform:'rotate(-2deg) scale(.94)',filter:'blur(8px)'}]},
    {in:[{opacity:0,transform:'scale(.9)',filter:'blur(22px) saturate(.6)'},{opacity:1,transform:'scale(1)',filter:'blur(0px) saturate(1)'}],
     out:[{opacity:1,transform:'scale(1)'},{opacity:0,transform:'scale(1.16)',filter:'blur(14px)'}]},
    {in:[{opacity:0,transform:'translate3d(0,-12%,0) scale(1.1)'},{opacity:1,transform:'translate3d(0,0,0) scale(1)'}],
     out:[{opacity:1,transform:'translate3d(0,0,0)'},{opacity:0,transform:'translate3d(0,12%,0)',filter:'blur(10px)'}]}
  ];
  var lastEffect = -1;
  function pickEffect(){
    var next = lastEffect;
    while (next === lastEffect) next = Math.floor(Math.random() * effects.length);
    lastEffect = next;
    return effects[next];
  }

  // 配音预加载：每镜一个 Audio 对象，播放失败时仍有兜底计时器推进。
  var audios = slides.map(function(slide){
    var src = slide.getAttribute('data-audio');
    if (!src) return null;
    var audio = new Audio();
    audio.preload = 'auto';
    audio.src = src;
    return audio;
  });

  var index = -1;
  var timer = 0;
  var token = 0;

  function clearTimer(){
    if (timer) { clearTimeout(timer); timer = 0; }
  }

  function show(nextIndex){
    if (nextIndex >= slides.length) { finish(); return; }
    token += 1;
    var my = token;
    clearTimer();

    var prev = index >= 0 ? slides[index] : null;
    var next = slides[nextIndex];
    var effect = pickEffect();
    var transitionMs = 1500;

    if (prev) {
      prev.getAnimations().forEach(function(anim){ anim.cancel(); });
      var out = prev.animate(effect.out, {duration: transitionMs * .86, easing: 'cubic-bezier(.4,0,.6,1)', fill: 'both'});
      out.onfinish = function(){ if (token === my) prev.classList.remove('is-active'); };
    }
    next.getAnimations().forEach(function(anim){ anim.cancel(); });
    next.classList.add('is-active');
    next.animate(effect.in, {duration: transitionMs, easing: 'cubic-bezier(.22,.61,.36,1)', fill: 'both'});
    index = nextIndex;

    if (countLabel) countLabel.textContent = (index + 1) + ' / ' + slides.length;
    if (bar) { bar.style.transition = 'none'; bar.style.width = '0%'; }

    var ms = parseInt(next.getAttribute('data-duration'), 10);
    if (!(ms > 0)) ms = 4000;

    var audio = audios[index];
    if (index > 0 && audios[index - 1]) audios[index - 1].pause();
    if (audio) {
      try { audio.currentTime = 0; } catch (err) {}
      var playing = audio.play();
      if (playing && playing.catch) playing.catch(function(){});
      audio.onended = function(){ if (token === my) show(index + 1); };
    }

    requestAnimationFrame(function(){
      if (token !== my || !bar) return;
      bar.style.transition = 'width ' + ms + 'ms linear';
      bar.style.width = '100%';
    });
    timer = setTimeout(function(){ if (token === my) show(index + 1); }, ms + 700);
    document.body.classList.add('playing');
  }

  function finish(){
    token += 1;
    clearTimer();
    if (bar) { bar.style.transition = 'none'; bar.style.width = '0%'; }
    document.body.classList.remove('playing');
    cover.classList.remove('is-hidden');
    if (startButton) startButton.textContent = '重播';
    var kicker = cover.querySelector('.kicker');
    if (kicker) kicker.textContent = 'THE END';
  }

  function restart(){
    token += 1;
    clearTimer();
    audios.forEach(function(audio){ if (audio) { try { audio.pause(); audio.currentTime = 0; } catch (err) {} } });
    slides.forEach(function(slide){ slide.classList.remove('is-active'); });
    index = -1;
    lastEffect = -1;
    cover.classList.add('is-hidden');
    show(0);
  }

  if (startButton) startButton.addEventListener('click', restart);
})();
</script>
</body>
</html>`
