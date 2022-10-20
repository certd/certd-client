import * as api from "./api";
import { dict } from "@fast-crud/fast-crud";
import { ref } from "vue";
import _ from "lodash-es";

export default function ({ expose, props, ctx }) {
  const { crudBinding } = expose;
  const lastResRef = ref();
  const pageRequest = async (query) => {
    return await api.GetList(query);
  };
  const editRequest = async ({ form, row }) => {
    form.id = row.id;
    form.type = props.type;
    const res = await api.UpdateObj(form);
    lastResRef.value = res;
    return res;
  };
  const delRequest = async ({ row }) => {
    return await api.DelObj(row.id);
  };

  const addRequest = async ({ form }) => {
    form.type = props.type;
    const res = await api.AddObj(form);
    lastResRef.value = res;
    return res;
  };
  let DNSProviderTypeDictRef = dict({
    url: "/certd/access/dnsProviderTypeDict"
  });
  let AccessTypeDictRef = dict({
    url: "/certd/access/accessTypeDict"
  });
  const selectedRowKey = ref([props.modelValue]);
  // watch(
  //   () => {
  //     return props.modelValue;
  //   },
  //   (value) => {
  //     selectedRowKey.value = [value];
  //   },
  //   {
  //     immediate: true
  //   }
  // );
  const onSelectChange = (changed) => {
    selectedRowKey.value = changed;
    ctx.emit("update:modelValue", changed[0]);
  };

  const typeRef = ref("aliyun");

  const defaultPluginConfig = {
    component: {
      name: "a-input",
      vModel: "value"
    }
  };
  function buildDefineFields(define, mode) {
    _.remove(crudBinding.value[mode + "Form"].columns, function (value, key) {
      return !!value._isUnstable;
    });
    _.forEach(define.input, (value, key) => {
      let field = {
        key,
        ...value,
        _isUnstable: true
      };
      crudBinding.value[mode + "Form"].columns[key] = _.merge({ title: key }, defaultPluginConfig, field);
      console.log("form", crudBinding.value[mode + "Form"]);
    });
  }
  return {
    typeRef,
    crudOptions: {
      request: {
        pageRequest,
        addRequest,
        editRequest,
        delRequest
      },
      toolbar: {
        show: false
      },
      search: {
        show: false
      },
      form: {
        wrapper: {
          width: "1050px"
        }
      },
      rowHandle: {
        width: "150px"
      },
      table: {
        rowSelection: {
          type: "radio",
          selectedRowKeys: selectedRowKey,
          onChange: onSelectChange
        },
        customRow: (record) => {
          return {
            onClick: () => {
              onSelectChange([record.id]);
            } // 点击行
          };
        }
      },
      columns: {
        id: {
          title: "ID",
          key: "id",
          type: "number",
          column: {
            width: 50
          },
          form: {
            show: false
          }
        },
        name: {
          title: "名称",
          search: {
            show: true
          },
          type: ["text"],
          form: {
            rules: [{ required: true, message: "请填写名称" }]
          }
        },
        type: {
          title: "类型",
          type: "dict-select",
          dict: AccessTypeDictRef,
          search: {
            show: false
          },
          form: {
            rules: [{ required: true, message: "请选择类型" }],
            component: {
              disabled: true
            },
            valueChange: {
              immediate: true,
              async handle({ value, mode }) {
                const define = await api.GetProviderDefine(value);
                console.log("define", define);
                buildDefineFields(define, mode);
              }
            }
          },
          addForm: {
            value: typeRef
          }
        },
        setting: {
          column: { show: false },
          form: {
            show: false,
            valueBuilder({ value, form }) {
              if (!value) {
                return;
              }
              const setting = JSON.parse(value);
              for (let key in setting) {
                form[key] = setting[key];
              }
            },
            valueResolve({ form }) {
              const setting = _.omit(form, "id", "name", "type", "setting");
              form.setting = JSON.stringify(setting);
            }
          }
        }
      }
    }
  };
}
