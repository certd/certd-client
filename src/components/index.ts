import ComponentRender from "./component-render.vue";
import DContainer from "./d-container.vue";
import AccessSelector from "./access-selector/index.vue";
export default {
  install(app) {
    app.component("ComponentRender", ComponentRender);
    app.component("DContainer", DContainer);
    app.component("AccessSelector", AccessSelector);
  }
};
