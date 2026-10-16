# Showcase

Speaker notes, one section per slide, headed with its title. The `post`
blocks make a Bluesky thread as the talk goes: moving on from a slide posts
its posts. A post before the first section opens the thread and attaches
nothing.

```post
Trying out live posting from a terminal slide deck: this thread posts itself as the talk goes. #decker https://github.com/austinlparker/decker
```

## Decker

A deck is a Go program. Everything you'll see is drawn into a pixel
canvas and shown as terminal cells.

```post
A slide deck that's a Go program, posting itself as it goes.
```
```alt
"DECKER" in stencil block letters, decrypting in, over "slide decks as Go programs".
```

## Statement

Two builds: the line, then the payoff. A view is a function of time, so
replay, snapshots and video all agree.

```post media=builds
Each build of a slide, as its own image.
```

## Letter effects

Effects are per glyph and compose with Chain. The clip below uses one of the
account's daily video uploads.

```post media=clip
Letter effects, as a looping clip.
```
```alt
The words Wave, Decode and DropIn animating in, beside "BLOCK FX" raining down in block letters.
```

## Numbers

Stats count up as they appear; the third waits for the second build.

```post media=none
One slide can post twice: this one first, text alone,
```

```post
then this one, with the slide.
```

## Charts

No post here: a slide without a post block doesn't post.

## Code

Each build moves the focus. The source is a plain string:

```go
decker.Code{Source: src, Lang: "go", Focus: focus}.Draw(c, p, rect)
```

```post media=builds builds=1,3
The first and last builds of a code walkthrough, picked with builds=1,3.
```
```alt
A Go function with line numbers, its focus moving down the lines.
```

## Diagram

Panels are placed by key, so the next slide can move them.

## Magic move

The morph glides each placed panel to its new place.

## Six builds

Six builds, six images: past four, a post goes up as a gallery.

```post media=builds
Six builds make a gallery.
```

## Film grain

Fine grain over a gradient makes a PNG over Bluesky's 2MB, so the image
goes up as a JPEG. Move on with `12g` to jump over the next slide.

```post
A slide too detailed for a PNG goes up as a JPEG.
```

## Skip me

Jumped over, this slide doesn't post, and the status line counts it as
skipped. Come back to it and move on, and it posts at the end of the
thread.

```post
This post goes out only if the slide wasn't skipped.
```

## Thanks

The last slide has nothing to move on to: it posts as soon as it's up.

```post
That's the thread. Thanks for following along.
```
