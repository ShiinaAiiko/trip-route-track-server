package methods

import (
	"math"
	"strings"

	"github.com/pkoukk/tiktoken-go"
)

func GetAIToolsParameterStr(parameter, typeStr, description string) string {
	return `"` + parameter + `": { "type": "` + typeStr + `", "description": "` + description + `"}`
}

func ContainsKeywords(input string, keywords ...string) bool {
	for _, word := range keywords {
		if strings.Contains(input, word) {
			return true
		}
	}
	return false
}

// CalculateCosineSimilarity 计算两个向量的余弦相似度
func CalculateCosineSimilarity(vecA, vecB []float32) float32 {
	// 1. 基础检查：长度不一致或为空则无意义
	if len(vecA) != len(vecB) || len(vecA) == 0 {
		return 0
	}

	// 2. 精度提升：使用 float64 进行中间计算，防止维度累加导致的精度截断
	var dotProduct, normA, normB float64

	for i := range vecA {
		a := float64(vecA[i])
		b := float64(vecB[i])

		dotProduct += a * b
		normA += a * a
		normB += b * b
	}

	// 3. 稳定性保护：防止分母为 0 导致的 NaN 或极小值导致的数值爆炸
	// 1e-10 是一个极其微小的浮点数阈值
	if normA < 1e-10 || normB < 1e-10 {
		return 0
	}

	// 4. 计算余弦值：(A·B) / (|A|*|B|)
	res := dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))

	// 5. 边界剪裁 (Clamping)：
	// 由于浮点数计算特性，结果可能出现 1.000000000001 这种越界值
	if res > 1.0 {
		res = 1.0
	} else if res < -1.0 {
		res = -1.0
	}

	return float32(res)
}

func GetTokenCount(text string) int {
	// OpenAI 大部分模型使用 cl100k_base 编码器
	tkm, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return 0
	}
	token := tkm.Encode(text, nil, nil)
	return len(token)
}
