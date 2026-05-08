import { createApp } from "vue";
import { createPinia } from "pinia";
import App from "./App.vue";
import router from "./router";
import { Alert } from "tdesign-vue-next/es/alert";
import { Button } from "tdesign-vue-next/es/button";
import { Checkbox, CheckboxGroup } from "tdesign-vue-next/es/checkbox";
import { Collapse, CollapsePanel } from "tdesign-vue-next/es/collapse";
import { ConfigProvider } from "tdesign-vue-next/es/config-provider";
import { Dialog } from "tdesign-vue-next/es/dialog";
import { Drawer } from "tdesign-vue-next/es/drawer";
import { Dropdown, DropdownItem, DropdownMenu } from "tdesign-vue-next/es/dropdown";
import { Empty } from "tdesign-vue-next/es/empty";
import { Form, FormItem } from "tdesign-vue-next/es/form";
import { Icon } from "tdesign-vue-next/es/icon";
import { ImageViewer } from "tdesign-vue-next/es/image-viewer";
import { Input } from "tdesign-vue-next/es/input";
import { InputNumber } from "tdesign-vue-next/es/input-number";
import { Link } from "tdesign-vue-next/es/link";
import { Loading } from "tdesign-vue-next/es/loading";
import { Option, OptionGroup, Select } from "tdesign-vue-next/es/select";
import { Popconfirm } from "tdesign-vue-next/es/popconfirm";
import { Popup } from "tdesign-vue-next/es/popup";
import { Progress } from "tdesign-vue-next/es/progress";
import { Radio, RadioButton, RadioGroup } from "tdesign-vue-next/es/radio";
import { Slider } from "tdesign-vue-next/es/slider";
import { Space } from "tdesign-vue-next/es/space";
import { Switch } from "tdesign-vue-next/es/switch";
import { TabPanel, Tabs } from "tdesign-vue-next/es/tabs";
import { Tag } from "tdesign-vue-next/es/tag";
import { Textarea } from "tdesign-vue-next/es/textarea";
import { Tooltip } from "tdesign-vue-next/es/tooltip";
import "@/assets/theme/theme.css";
import "@/assets/dropdown-menu.less";
import i18n, { initI18nLocale } from "./i18n";
import { initTheme } from "@/composables/useTheme";

initTheme();

const bootstrap = async () => {
  await initI18nLocale();

  const app = createApp(App);

  [
    Alert,
    Button,
    Checkbox,
    CheckboxGroup,
    Collapse,
    CollapsePanel,
    ConfigProvider,
    Dialog,
    Drawer,
    Dropdown,
    DropdownItem,
    DropdownMenu,
    Empty,
    Form,
    FormItem,
    Icon,
    ImageViewer,
    Input,
    InputNumber,
    Link,
    Loading,
    Option,
    OptionGroup,
    Popconfirm,
    Popup,
    Progress,
    Radio,
    RadioButton,
    RadioGroup,
    Select,
    Slider,
    Space,
    Switch,
    TabPanel,
    Tabs,
    Tag,
    Textarea,
    Tooltip,
  ].forEach((component) => {
    app.use(component);
  });
  app.use(createPinia());
  app.use(router);
  app.use(i18n);

  app.mount("#app");
};

bootstrap();
