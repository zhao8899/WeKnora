//go:build cgo

package types

import "github.com/yanyiwu/gojieba"

type jiebaSegmenter struct {
	impl *gojieba.Jieba
}

func newWordSegmenter() WordSegmenter {
	return &jiebaSegmenter{impl: gojieba.NewJieba()}
}

func (j *jiebaSegmenter) Cut(text string, hmm bool) []string {
	return j.impl.Cut(text, hmm)
}

func (j *jiebaSegmenter) CutForSearch(text string, hmm bool) []string {
	return j.impl.CutForSearch(text, hmm)
}
