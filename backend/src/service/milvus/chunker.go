package milvus

// SemanticChunks 将文本按字符窗口切块，优先在换行和中文标点处断句，块间带重叠。
// 对标 Python server.py 中的 semantic_chunks() 函数。
func SemanticChunks(text string, maxChars, overlap int) []string {
	if maxChars <= 0 {
		maxChars = 420
	}
	if overlap < 0 {
		overlap = 72
	}
	t := []rune(text)
	n := len(t)
	if n == 0 {
		return nil
	}
	if n <= maxChars {
		return []string{string(t)}
	}

	punct := map[rune]bool{'\n': true, '。': true, '！': true, '？': true, '；': true, '，': true, '、': true}

	var out []string
	start := 0
	for start < n {
		end := start + maxChars
		if end > n {
			end = n
		}
		if end < n {
			best := end
			scanLo := start + max(1, maxChars-120)
			for i := end - 1; i >= scanLo; i-- {
				if punct[t[i]] {
					best = i + 1
					break
				}
			}
			end = best
		}
		chunk := []rune{}
		for _, r := range t[start:end] {
			if r != ' ' || len(chunk) > 0 {
				chunk = append(chunk, r)
			}
		}
		// trim leading/trailing spaces
		chunk = trimRunes(chunk)
		if len(chunk) > 0 {
			out = append(out, string(chunk))
		}
		if end >= n {
			break
		}
		next := end - overlap
		if next <= start {
			next = start + 1
		}
		start = next
	}
	if len(out) == 0 {
		s := string(t)
		if maxChars < n {
			s = string(t[:maxChars])
		}
		return []string{s}
	}
	return out
}

// BuildFullEmbedText 拼接标题和正文（对标 Python build_full_embed_text）。
func BuildFullEmbedText(title, content string, maxContentRunes int) string {
	if maxContentRunes <= 0 {
		maxContentRunes = 2500
	}
	t := trimStr(title)
	c := trimStr(clip(content, maxContentRunes))
	if c == "" {
		return t
	}
	if t == "" {
		return c
	}
	return t + "\n" + c
}

// EmbedChunkText 在 chunk 文本前加标题前缀（对标 Python embed_chunk_text）。
func EmbedChunkText(title, chunk string) string {
	t := trimStr(title)
	c := trimStr(chunk)
	if t == "" {
		return c
	}
	return t + "\n" + c
}

func trimRunes(r []rune) []rune {
	start := 0
	for start < len(r) && (r[start] == ' ' || r[start] == '\t') {
		start++
	}
	end := len(r)
	for end > start && (r[end-1] == ' ' || r[end-1] == '\t') {
		end--
	}
	return r[start:end]
}

func trimStr(s string) string {
	r := trimRunes([]rune(s))
	return string(r)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
