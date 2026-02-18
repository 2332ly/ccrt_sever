package config

import (
	"encoding/json"
	"errors"
	"strings"

	"ccrt_sever/global"
	"ccrt_sever/models"
)

func ensureMMSESeed() error {
	var count int64
	if err := global.Db.Model(&models.Scale{}).Where("name = ?", "MMSE").Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	scale := models.Scale{
		Name:       "MMSE",
		TotalScore: 30,
	}
	if err := global.Db.Create(&scale).Error; err != nil {
		return err
	}

	version := models.ScaleVersion{
		ScaleID:    scale.ID,
		Name:       "MMSE",
		Version:    "2001",
		TotalScore: 30,
		IsActive:   true,
	}
	if err := global.Db.Create(&version).Error; err != nil {
		return err
	}

	modules := []struct {
		Name      string
		MaxScore  int
		SortOrder int
		Questions []struct {
			Type      string
			Content   string
			MaxScore  int
			SortOrder int
			Rule      any
		}
	}{
		{
			Name:      "定向力",
			MaxScore:  10,
			SortOrder: 1,
			Questions: []struct {
				Type      string
				Content   string
				MaxScore  int
				SortOrder int
				Rule      any
			}{
				{
					Type:      "fields_correct",
					Content:   "现在是？年份/月份/日期/星期/季节（每项1分）",
					MaxScore:  5,
					SortOrder: 1,
					Rule: map[string]any{
						"type": "fields_correct",
						"fields": []map[string]any{
							{"key": "year", "label": "年份"},
							{"key": "month", "label": "月份"},
							{"key": "day", "label": "日期"},
							{"key": "weekday", "label": "星期"},
							{"key": "season", "label": "季节"},
						},
						"score_per_field": 1,
					},
				},
				{
					Type:      "fields_correct",
					Content:   "我们现在在哪里？省市/区县/街道/具体地点/楼层（每项1分）",
					MaxScore:  5,
					SortOrder: 2,
					Rule: map[string]any{
						"type": "fields_correct",
						"fields": []map[string]any{
							{"key": "province", "label": "省市"},
							{"key": "district", "label": "区县"},
							{"key": "street", "label": "街道"},
							{"key": "location", "label": "具体地点"},
							{"key": "floor", "label": "楼层"},
						},
						"score_per_field": 1,
					},
				},
			},
		},
		{
			Name:      "记忆力",
			MaxScore:  3,
			SortOrder: 2,
			Questions: []struct {
				Type      string
				Content   string
				MaxScore  int
				SortOrder int
				Rule      any
			}{
				{
					Type:      "set_match",
					Content:   "记住并立即复述：花园、冰箱、国旗",
					MaxScore:  3,
					SortOrder: 1,
					Rule: map[string]any{
						"type":           "set_match",
						"correct_set":    []string{"花园", "冰箱", "国旗"},
						"score_per_item": 1,
					},
				},
			},
		},
		{
			Name:      "注意力和计算力",
			MaxScore:  5,
			SortOrder: 3,
			Questions: []struct {
				Type      string
				Content   string
				MaxScore  int
				SortOrder int
				Rule      any
			}{
				{
					Type:      "sequence_match",
					Content:   "从100开始每次减7，给出5个答案",
					MaxScore:  5,
					SortOrder: 1,
					Rule: map[string]any{
						"type":             "sequence_match",
						"correct_sequence": []int{93, 86, 79, 72, 65},
						"score_per_step":   1,
						"allow_partial":    true,
					},
				},
			},
		},
		{
			Name:      "回忆力",
			MaxScore:  3,
			SortOrder: 4,
			Questions: []struct {
				Type      string
				Content   string
				MaxScore  int
				SortOrder int
				Rule      any
			}{
				{
					Type:      "set_match",
					Content:   "回忆刚才那三个词语：花园、冰箱、国旗",
					MaxScore:  3,
					SortOrder: 1,
					Rule: map[string]any{
						"type":           "set_match",
						"correct_set":    []string{"花园", "冰箱", "国旗"},
						"score_per_item": 1,
					},
				},
			},
		},
		{
			Name:      "语言能力",
			MaxScore:  9,
			SortOrder: 5,
			Questions: []struct {
				Type      string
				Content   string
				MaxScore  int
				SortOrder int
				Rule      any
			}{
				{
					Type:      "manual",
					Content:   "命名：手表",
					MaxScore:  1,
					SortOrder: 1,
					Rule: map[string]any{
						"type": "manual",
					},
				},
				{
					Type:      "manual",
					Content:   "命名：铅笔",
					MaxScore:  1,
					SortOrder: 2,
					Rule: map[string]any{
						"type": "manual",
					},
				},
				{
					Type:      "exact_text",
					Content:   "复述：四十四只石狮子",
					MaxScore:  1,
					SortOrder: 3,
					Rule: map[string]any{
						"type":        "exact_text",
						"expected":    "四十四只石狮子",
						"score_value": 1,
					},
				},
				{
					Type:      "multi_step",
					Content:   "三步指令：用右手拿纸/对折/放左腿",
					MaxScore:  3,
					SortOrder: 4,
					Rule: map[string]any{
						"type":           "multi_step",
						"steps":          []string{"用右手拿纸", "对折", "放左腿"},
						"score_per_step": 1,
					},
				},
				{
					Type:      "manual",
					Content:   "阅读并执行：闭上你的眼睛",
					MaxScore:  1,
					SortOrder: 5,
					Rule: map[string]any{
						"type": "manual",
					},
				},
				{
					Type:      "manual",
					Content:   "写一个完整句子",
					MaxScore:  1,
					SortOrder: 6,
					Rule: map[string]any{
						"type": "manual",
					},
				},
				{
					Type:      "manual",
					Content:   "临摹图形",
					MaxScore:  1,
					SortOrder: 7,
					Rule: map[string]any{
						"type": "manual",
					},
				},
			},
		},
	}

	for _, m := range modules {
		module := models.ScaleModule{
			ScaleVersionID: version.ID,
			Name:           m.Name,
			MaxScore:       m.MaxScore,
			SortOrder:      m.SortOrder,
		}
		if err := global.Db.Create(&module).Error; err != nil {
			return err
		}
		for _, q := range m.Questions {
			ruleJSON, err := json.Marshal(q.Rule)
			if err != nil {
				return err
			}
			ruleStr := strings.TrimSpace(string(ruleJSON))
			if ruleStr == "" {
				return errors.New("empty answer rule")
			}
			question := models.ScaleQuestion{
				ModuleID:   module.ID,
				Type:       q.Type,
				Content:    q.Content,
				MaxScore:   q.MaxScore,
				AnswerRule: ruleStr,
				SortOrder:  q.SortOrder,
			}
			if err := global.Db.Create(&question).Error; err != nil {
				return err
			}
		}
	}

	return nil
}
