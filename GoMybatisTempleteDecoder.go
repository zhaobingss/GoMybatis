package GoMybatis

import (
	"GoMybatis/utils"
	"bytes"
	"github.com/beevik/etree"
	"reflect"
	"strings"
)

var equalOperator = []string{"/", "+", "-", "*", "**", "|", "^", "&", "%", "<", ">", ">=", "<=", " in ", " not in ", " or ", "||", " and ", "&&", "==", "!="}

/**
TODO sqlTemplete解析器，目前直接操作*etree.Element实现，后期应该改成操作xml，换取更好的维护性
*/
type GoMybatisTempleteDecoder struct {
}

type LogicDeleteData struct {
	Column   string
	Property string
	LangType string

	Enable         bool
	Deleted_value  string
	Undelete_value string
}

type VersionData struct {
	Column   string
	Property string
	LangType string
}

func (it *GoMybatisTempleteDecoder) DecodeTree(tree map[string]etree.Token, beanType reflect.Type) error {
	if tree == nil {
		return utils.NewError("GoMybatisTempleteDecoder", "decode data map[string]*MapperXml cant be nil!")
	}
	if beanType != nil {
		if beanType.Kind() == reflect.Ptr {
			beanType = beanType.Elem()
		}
	}
	for _, item := range tree {
		var typeString = reflect.TypeOf(item).String()
		if typeString == "*etree.Element" {
			var v = item.(*etree.Element)
			var method *reflect.StructField
			if beanType != nil {
				if isMethodElement(v.Tag) {
					var upperId = utils.UpperFieldFirstName(v.SelectAttrValue("id", ""))
					m, haveMethod := beanType.FieldByName(upperId)
					if haveMethod {
						method = &m
					}
				}
			}
			it.Decode(method, v, tree)
		}
	}
	return nil
}

func (it *GoMybatisTempleteDecoder) Decode(method *reflect.StructField, mapper *etree.Element, tree map[string]etree.Token) error {

	switch mapper.Tag {

	case "selectTemplete":
		mapper.Tag = Element_Select

		var id = mapper.SelectAttrValue("id", "")
		if id == "" {
			mapper.CreateAttr("id", "selectTemplete")
		}

		var tables = mapper.SelectAttrValue("tables", "")
		var columns = mapper.SelectAttrValue("columns", "")
		var wheres = mapper.SelectAttrValue("wheres", "")

		var resultMap = mapper.SelectAttrValue("resultMap", "")
		if resultMap == "" {
			resultMap = "BaseResultMap"
		}
		var resultMapData = tree[resultMap].(*etree.Element)
		if resultMapData == nil {
			panic(utils.NewError("GoMybatisTempleteDecoder", "resultMap not define! id = ", resultMap))
		}
		checkTablesValue(mapper, &tables, resultMapData)

		var logic = it.decodeLogicDelete(resultMapData)

		var sql bytes.Buffer
		sql.WriteString("select ")
		if columns == "" {
			columns = "*"
		}
		sql.WriteString(columns)
		sql.WriteString(" from ")
		sql.WriteString(tables)
		if len(wheres) > 0 {
			sql.WriteString(" where ")
			mapper.Child = append(mapper.Child, &etree.CharData{
				Data: sql.String(),
			})
			//TODO decode wheres
			it.DecodeWheres(wheres, mapper, logic, nil)
		}
		break
	case "insertTemplete":
		mapper.Tag = Element_Insert

		var id = mapper.SelectAttrValue("id", "")
		if id == "" {
			mapper.CreateAttr("id", "insertTemplete")
		}

		var tables = mapper.SelectAttrValue("tables", "")
		var inserts = mapper.SelectAttrValue("inserts", "")

		var resultMap = mapper.SelectAttrValue("resultMap", "")
		if resultMap == "" {
			resultMap = "BaseResultMap"
		}
		if inserts == "" {
			inserts = "*?*"
		}

		var resultMapData = tree[resultMap].(*etree.Element)
		if resultMapData == nil {
			panic(utils.NewError("GoMybatisTempleteDecoder", "resultMap not define! id = ", resultMap))
		}
		checkTablesValue(mapper, &tables, resultMapData)

		var logic = it.decodeLogicDelete(resultMapData)

		var collection string
		//check method arg type
		if method != nil {
			method.Type.NumIn()
			for i := 0; i < method.Type.NumIn(); i++ {
				var itemType = method.Type.In(i)
				if itemType.Kind() == reflect.Slice {
					var mapperParams = method.Tag.Get("mapperParams")
					var args = strings.Split(mapperParams, ",")
					collection = args[i]
				}
			}
		}

		//start builder
		var sql bytes.Buffer
		sql.WriteString("insert into ")
		sql.WriteString(tables)

		mapper.Child = append(mapper.Child, &etree.CharData{
			Data: sql.String(),
		})

		//add insert column
		var trimColumn = etree.Element{
			Tag: Element_Trim,
			Attr: []etree.Attr{
				{Key: "prefix", Value: "("},
				{Key: "suffix", Value: ")"},
				{Key: "suffixOverrides", Value: ","},
			},
			Child: []etree.Token{},
		}

		//cloumns
		if collection != "" {
			for _, v := range resultMapData.ChildElements() {
				if inserts == "*" || inserts == "*?*" {
					trimColumn.Child = append(trimColumn.Child, &etree.CharData{
						Data: v.SelectAttrValue("column", "") + ",",
					})
				}
			}
		} else {
			for _, v := range resultMapData.ChildElements() {
				if collection == "" && inserts == "*?*" {
					trimColumn.Child = append(trimColumn.Child, &etree.Element{
						Tag: Element_If,
						Attr: []etree.Attr{
							{Key: "test", Value: it.makeIfNotNull(v.SelectAttrValue("property", ""))},
						},
						Child: []etree.Token{
							&etree.CharData{
								Data: v.SelectAttrValue("column", "") + ",",
							},
						},
					})
				} else if inserts == "*" {
					trimColumn.Child = append(trimColumn.Child, &etree.CharData{
						Data: v.SelectAttrValue("column", "") + ",",
					})
				}
			}
		}

		mapper.Child = append(mapper.Child, &trimColumn)

		//args
		var trimArg = etree.Element{
			Tag:   Element_Trim,
			Attr:  []etree.Attr{{Key: "prefix", Value: "values ("}, {Key: "suffix", Value: ")"}, {Key: "suffixOverrides", Value: ","}},
			Child: []etree.Token{},
		}

		if collection == "" {
			for _, v := range resultMapData.ChildElements() {
				if logic.Enable && v.SelectAttrValue("property", "") == logic.Property {
					trimArg.Child = append(trimArg.Child, &etree.CharData{
						Data: logic.Undelete_value + ",",
					})
					continue
				}
				if inserts == "*?*" {
					trimArg.Child = append(trimArg.Child, &etree.Element{
						Tag:  Element_If,
						Attr: []etree.Attr{{Key: "test", Value: it.makeIfNotNull(v.SelectAttrValue("property", ""))}},
						Child: []etree.Token{
							&etree.CharData{
								Data: "#{" + v.SelectAttrValue("property", "") + "},",
							},
						},
					})
				} else if inserts == "*" {
					trimArg.Child = append(trimArg.Child, &etree.CharData{
						Data: "#{" + v.SelectAttrValue("property", "") + "},",
					})
				}
			}
		} else {
			trimArg.Attr[0] = etree.Attr{Key: "prefix", Value: "values "}
			trimArg.Attr[1] = etree.Attr{Key: "suffix", Value: ""}
			trimArg.Attr[2] = etree.Attr{Key: "suffixOverrides", Value: ","}

			var forEach = &etree.Element{
				Tag:   Element_Foreach,
				Attr:  []etree.Attr{{Key: "open", Value: ""}, {Key: "close", Value: ""}, {Key: "separator", Value: ","}, {Key: "collection", Value: collection}},
				Child: []etree.Token{},
			}

			for index, v := range resultMapData.ChildElements() {
				var prefix = ""
				if index == 0 {
					prefix = "("
				}
				var value = prefix + "#{" + "item." + utils.UpperFieldFirstName(v.SelectAttrValue("property", "")) + "}"
				if logic.Enable && v.SelectAttrValue("property", "") == logic.Property {
					value = `'` + logic.Undelete_value + "'"
				}
				if index+1 == len(resultMapData.Child) {
					value += ")"
				} else {
					value += ","
				}
				forEach.Child = append(forEach.Child, &etree.CharData{
					Data: value,
				})
			}
			trimArg.Child = append(trimArg.Child, forEach)
		}
		mapper.Child = append(mapper.Child, &trimArg)

		break
	case "updateTemplete":
		mapper.Tag = Element_Update

		var id = mapper.SelectAttrValue("id", "")
		if id == "" {
			mapper.CreateAttr("id", "updateTemplete")
		}

		var tables = mapper.SelectAttrValue("tables", "")
		var columns = mapper.SelectAttrValue("sets", "")
		var wheres = mapper.SelectAttrValue("wheres", "")

		var resultMap = mapper.SelectAttrValue("resultMap", "")
		if resultMap == "" {
			resultMap = "BaseResultMap"
		}

		var resultMapData = tree[resultMap].(*etree.Element)
		if resultMapData == nil {
			panic(utils.NewError("GoMybatisTempleteDecoder", "resultMap not define! id = ", resultMap))
		}
		checkTablesValue(mapper, &tables, resultMapData)

		var logic = it.decodeLogicDelete(resultMapData)

		var versionData = it.decodeVersionData(resultMapData)

		var sql bytes.Buffer
		sql.WriteString("update ")
		sql.WriteString(tables)
		sql.WriteString(" set ")
		if columns == "" {
			mapper.Child = append(mapper.Child, &etree.CharData{
				Data: sql.String(),
			})
			sql.Reset()
			for _, v := range resultMapData.ChildElements() {
				columns += v.SelectAttrValue("property", "") + "?" + v.SelectAttrValue("column", "") + ","
			}
			columns = strings.Trim(columns, ",")
			it.DecodeSets(columns, mapper, LogicDeleteData{}, versionData)
		} else {
			mapper.Child = append(mapper.Child, &etree.CharData{
				Data: sql.String(),
			})
			sql.Reset()
			it.DecodeSets(columns, mapper, LogicDeleteData{}, versionData)
		}
		if len(wheres) > 0 || logic.Enable {
			sql.WriteString(" where ")
			mapper.Child = append(mapper.Child, &etree.CharData{
				Data: sql.String(),
			})
			it.DecodeWheres(wheres, mapper, logic, versionData)
		}
		break
	case "deleteTemplete":
		mapper.Tag = Element_Delete

		var id = mapper.SelectAttrValue("id", "")
		if id == "" {
			mapper.CreateAttr("id", "deleteTemplete")
		}

		var tables = mapper.SelectAttrValue("tables", "")
		var wheres = mapper.SelectAttrValue("wheres", "")

		var resultMap = mapper.SelectAttrValue("resultMap", "")
		if resultMap == "" {
			resultMap = "BaseResultMap"
		}

		var resultMapData = tree[resultMap].(*etree.Element)
		if resultMapData == nil {
			panic(utils.NewError("GoMybatisTempleteDecoder", "resultMap not define! id = ", resultMap))
		}
		checkTablesValue(mapper, &tables, resultMapData)

		var logic = it.decodeLogicDelete(resultMapData)
		if logic.Enable {
			//enable logic delete
			var sql bytes.Buffer
			sql.WriteString("update ")
			sql.WriteString(tables)
			sql.WriteString(" set ")
			mapper.Child = append(mapper.Child, &etree.CharData{
				Data: sql.String(),
			})
			sql.Reset()
			it.DecodeSets("", mapper, logic, nil)
			if len(wheres) > 0 {
				sql.WriteString(" where ")
				mapper.Child = append(mapper.Child, &etree.CharData{
					Data: sql.String(),
				})
				//TODO decode wheres
				it.DecodeWheres(wheres, mapper, logic, nil)
			}
			break
		} else {
			//default delete  DELETE FROM `test`.`biz_activity` WHERE `id`='165';
			var sql bytes.Buffer
			sql.WriteString("delete from ")
			sql.WriteString(tables)
			if len(wheres) > 0 {
				sql.WriteString(" where ")
				mapper.Child = append(mapper.Child, &etree.CharData{
					Data: sql.String(),
				})
				//TODO decode wheres
				it.DecodeWheres(wheres, mapper, LogicDeleteData{}, nil)
			}
		}
	}

	return nil
}

func checkTablesValue(mapper *etree.Element, tables *string, resultMapData *etree.Element) {
	if *tables == "" {
		*tables = resultMapData.SelectAttrValue("tables", "")
		if *tables == "" {
			panic("[GoMybatisTempleteDecoder] attribute 'tables' can not be empty! need define in <resultMap> or <" + mapper.Tag + "Templete>,mapper id=" + mapper.SelectAttrValue("id", ""))
		}
	}
}

//解码逗号分隔的where
func (it *GoMybatisTempleteDecoder) DecodeWheres(arg string, mapper *etree.Element, logic LogicDeleteData, versionData *VersionData) {
	var wheres = strings.Split(arg, ",")
	for index, v := range wheres {
		var expressions = strings.Split(v, "?")
		if len(expressions) > 1 {
			//TODO have ?
			var newWheres bytes.Buffer
			if index > 0 {
				newWheres.WriteString(" and ")
			}
			newWheres.WriteString(expressions[1])
			var item = &etree.Element{
				Tag:   Element_If,
				Attr:  []etree.Attr{{Key: "test", Value: it.makeIfNotNull(expressions[0])}},
				Child: []etree.Token{&etree.CharData{Data: newWheres.String()}},
			}
			mapper.Child = append(mapper.Child, item)
		} else {
			var newWheres bytes.Buffer
			if index > 0 {
				newWheres.WriteString(" and ")
			}
			newWheres.WriteString(v)
			var item = &etree.CharData{
				Data: newWheres.String(),
			}
			mapper.Child = append(mapper.Child, item)
		}
	}
	if logic.Enable == true {
		var appendAdd = ""
		if len(wheres) >= 1 && arg != "" {
			appendAdd = " and "
		}
		var item = &etree.CharData{
			Data: appendAdd + logic.Column + " = " + logic.Undelete_value,
		}
		mapper.Child = append(mapper.Child, item)
	}
	if versionData != nil {
		var appendAdd = ""
		if len(wheres) >= 1 && arg != "" {
			appendAdd = " and "
		}
		var item = &etree.CharData{
			Data: appendAdd + versionData.Column + " = #{" + versionData.Property + "}",
		}
		mapper.Child = append(mapper.Child, item)
	}
}

func (it *GoMybatisTempleteDecoder) DecodeSets(arg string, mapper *etree.Element, logic LogicDeleteData, versionData *VersionData) {
	var sets = strings.Split(arg, ",")
	for index, v := range sets {
		var expressions = strings.Split(v, "?")
		if len(expressions) > 1 {
			//TODO have ?
			var newWheres bytes.Buffer
			if index > 0 {
				newWheres.WriteString(",")
			}
			newWheres.WriteString(expressions[1])
			var item = &etree.Element{
				Tag:  Element_If,
				Attr: []etree.Attr{{Key: "test", Value: it.makeIfNotNull(expressions[0])}},
			}
			item.SetText(newWheres.String())
			mapper.Child = append(mapper.Child, item)
		} else {
			var newWheres bytes.Buffer
			if index > 0 {
				newWheres.WriteString(" and ")
			}
			newWheres.WriteString(v)
			var item = &etree.CharData{
				Data: newWheres.String(),
			}
			mapper.Child = append(mapper.Child, item)
		}
	}
	if logic.Enable == true {
		var appendAdd = ""
		if len(sets) >= 1 && arg != "" {
			appendAdd = ","
		}
		var item = &etree.CharData{
			Data: appendAdd + logic.Column + " = " + logic.Deleted_value,
		}
		mapper.Child = append(mapper.Child, item)
	}
	if versionData != nil {
		var appendAdd = ""
		if len(sets) >= 1 && arg != "" {
			appendAdd = ","
		}
		var item = &etree.CharData{
			Data: appendAdd + versionData.Column + " = #{" + versionData.Property + "+1}",
		}
		mapper.Child = append(mapper.Child, item)
	}
}

func (it *GoMybatisTempleteDecoder) makeIfNotNull(arg string) string {
	for _, v := range equalOperator {
		if strings.Contains(arg, v) {
			return arg
		}
	}
	return arg + ` != nil`
}

func (it *GoMybatisTempleteDecoder) decodeLogicDelete(xml *etree.Element) LogicDeleteData {
	if xml == nil || len(xml.Child) == 0 {
		return LogicDeleteData{}
	}
	var logicData = LogicDeleteData{}
	for _, v := range xml.ChildElements() {
		if v.SelectAttrValue("logic_enable", "") == "true" {
			logicData.Enable = true
			logicData.Deleted_value = v.SelectAttrValue("logic_deleted", "")
			logicData.Undelete_value = v.SelectAttrValue("logic_undelete", "")
			logicData.Column = v.SelectAttrValue("column", "")
			logicData.Property = v.SelectAttrValue("property", "")
			logicData.LangType = v.SelectAttrValue("langType", "")
			//check
			if logicData.Deleted_value == "" {
				panic(utils.NewError("GoMybatisTempleteDecoder", `<resultMap> logic_deleted="" can't be empty !`))
			}
			if logicData.Undelete_value == "" {
				panic(utils.NewError("GoMybatisTempleteDecoder", `<resultMap> logic_undelete="" can't be empty !`))
			}
			if logicData.Undelete_value == logicData.Deleted_value {
				panic(utils.NewError("GoMybatisTempleteDecoder", `<resultMap> logic_deleted value can't be logic_undelete value!`))
			}
			break
		}
	}
	return logicData
}

func (it *GoMybatisTempleteDecoder) decodeVersionData(xml *etree.Element) *VersionData {
	if xml == nil || len(xml.Child) == 0 {
		return nil
	}
	for _, v := range xml.ChildElements() {
		if v.SelectAttrValue("version_enable", "") == "true" {

			var versionData = VersionData{}
			versionData.Column = v.SelectAttrValue("column", "")
			versionData.Property = v.SelectAttrValue("property", "")
			versionData.LangType = v.SelectAttrValue("langType", "")
			//check
			if !(strings.Contains(versionData.LangType, "int") || strings.Contains(versionData.LangType, "time.Time")) {
				panic(utils.NewError("GoMybatisTempleteDecoder", `version_enable only support int...,time.Time... number type!`))
			}
			return &versionData
		}
	}
	return nil
}
