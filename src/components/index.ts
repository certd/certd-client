import ComponentRender from "./component-render.vue";
import DContainer from "./d-container.vue";
import AccessSelector from "./access-selector/index.vue";
import HoverEdit from "./hover-edit.vue";
import { CheckCircleOutlined, InfoCircleOutlined, UndoOutlined } from "@ant-design/icons-vue";
export default {
  install(app) {
    app.component("ComponentRender", ComponentRender);
    app.component("DContainer", DContainer);
    app.component("AccessSelector", AccessSelector);
    app.component("HoverEdit", HoverEdit);

    app.component("CheckCircleOutlined", CheckCircleOutlined);
    app.component("InfoCircleOutlined", InfoCircleOutlined);
    app.component("UndoOutlined", UndoOutlined);
  }
};
