let map;
const pathCoords = [
    [35.9119467, 127.0659829], // A
    [35.9121794, 127.0665127], // B
    [35.9126749, 127.0685961], // C
    [35.9117154, 127.0666587]  // D
];

function initMap() {
    console.log("正在使用 Leaflet 初始化地图...");
    
    // 1. 初始化地图并设置中心点
    map = L.map('map').setView(pathCoords[0], 13);

    // 2. 加载地图瓦片 (OpenStreetMap 开源底图，不需要 Key)
    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
        attribution: '© OpenStreetMap contributors'
    }).addTo(map);

    // 3. 画红色连线
    L.polyline(pathCoords, {color: 'red', weight: 3}).addTo(map);

    // 4. 打点 A, B, C, D
    const labels = ['A', 'B', 'C', 'D'];
    pathCoords.forEach((coord, index) => {
        L.marker(coord).addTo(map)
            .bindPopup("位置: " + labels[index])
            .bindTooltip(labels[index], {permanent: true, direction: 'right'});
    });
}

// 页面加载后立即启动地图
window.addEventListener('DOMContentLoaded', initMap);
